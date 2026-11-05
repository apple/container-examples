// config_gate_test.go asserts one property: every manifest under config/ decodes strictly
// and wires up: selectors match their pods, Services and policies point at the right
// labels and ports.
// Kubernetes kinds decode into k8s.io/api types.
// The one Cilium kind, allow-lb, decodes into ciliumNetworkPolicy below, which holds only
// the fields allow-lb may use, so an egress section or any other field fails the test.
package controller

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/yaml"
)

// ciliumNetworkPolicy is the subset of cilium.io/v2 CiliumNetworkPolicy that allow-lb uses.
// The field names and JSON tags follow the Cilium API (pkg/policy/api in cilium/cilium).
type ciliumNetworkPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              struct {
		EndpointSelector metav1.LabelSelector `json:"endpointSelector"`
		Ingress          []struct {
			FromEntities []string `json:"fromEntities"`
			ToPorts      []struct {
				Ports []struct {
					Port     string `json:"port"`
					Protocol string `json:"protocol"`
				} `json:"ports"`
			} `json:"toPorts"`
		} `json:"ingress"`
	} `json:"spec"`
}

func decodeConfig(t *testing.T) map[string]any {
	t.Helper()
	decoder := serializer.NewCodecFactory(clientgoscheme.Scheme, serializer.EnableStrict).UniversalDeserializer()
	objs := map[string]any{}
	err := filepath.Walk(filepath.Join("..", "..", "config"), func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if filepath.Base(path) == "kustomization.yaml" {
			var k map[string]any // not a Kubernetes type; it must still be valid YAML
			if err := yaml.Unmarshal(b, &k); err != nil || k["kind"] != "Kustomization" {
				t.Errorf("%s: %v", path, err)
			}
			return nil
		}
		r := utilyaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(b)))
		for {
			doc, err := r.Read()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
			if len(bytes.TrimSpace(doc)) == 0 {
				continue
			}
			var tm metav1.TypeMeta
			if err := yaml.Unmarshal(doc, &tm); err != nil {
				t.Errorf("%s: %v", path, err)
				continue
			}
			if tm.APIVersion == "cilium.io/v2" && tm.Kind == "CiliumNetworkPolicy" {
				var cnp ciliumNetworkPolicy
				if err := yaml.UnmarshalStrict(doc, &cnp); err != nil {
					t.Errorf("%s: %v", path, err)
					continue
				}
				objs[cnp.Kind+"/"+cnp.Name] = &cnp
				continue
			}
			obj, gvk, err := decoder.Decode(doc, nil, nil)
			if err != nil {
				t.Errorf("%s: %v", path, err)
				continue
			}
			objs[gvk.Kind+"/"+obj.(interface{ GetName() string }).GetName()] = obj
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	return objs
}

func matches(sel, labels map[string]string) bool {
	if len(sel) == 0 {
		return false
	}
	for k, v := range sel {
		if labels[k] != v {
			return false
		}
	}
	return true
}

func TestConfigWiresUp(t *testing.T) {
	objs := decodeConfig(t)
	get := func(key string) any {
		o, ok := objs[key]
		if !ok {
			t.Fatalf("%s missing from config/", key)
		}
		return o
	}
	app := get("Deployment/sample-app").(*appsv1.Deployment)
	pg := get("Deployment/postgres").(*appsv1.Deployment)
	hello := get("Deployment/hello").(*appsv1.Deployment)
	for _, d := range []*appsv1.Deployment{app, pg, hello} {
		if !matches(d.Spec.Selector.MatchLabels, d.Spec.Template.Labels) {
			t.Errorf("%s: selector %v does not match its pods %v", d.Name, d.Spec.Selector.MatchLabels, d.Spec.Template.Labels)
		}
	}
	appC, pgC, helloC := app.Spec.Template.Spec.Containers[0], pg.Spec.Template.Spec.Containers[0], hello.Spec.Template.Spec.Containers[0]
	if appC.Image != "sample-app:dev" || appC.ImagePullPolicy != corev1.PullNever {
		t.Errorf("app image %s %s", appC.Image, appC.ImagePullPolicy)
	}
	if pgC.Image != "postgres:17.6-alpine" || helloC.Image != "registry.k8s.io/pause:3.10.1" {
		t.Errorf("pinned images: %s, %s", pgC.Image, helloC.Image)
	}
	if *hello.Spec.Replicas != 1 || *pg.Spec.Replicas != 1 {
		t.Errorf("replicas: hello %d, postgres %d", *hello.Spec.Replicas, *pg.Spec.Replicas)
	}

	svc := get("Service/sample-app").(*corev1.Service)
	if svc.Spec.Type != corev1.ServiceTypeLoadBalancer || !matches(svc.Spec.Selector, app.Spec.Template.Labels) ||
		svc.Spec.Ports[0].Port != 80 || svc.Spec.Ports[0].TargetPort.StrVal != appC.Ports[0].Name {
		t.Errorf("app Service = %+v", svc.Spec)
	}
	db := get("Service/postgres").(*corev1.Service)
	if !matches(db.Spec.Selector, pg.Spec.Template.Labels) || db.Spec.Ports[0].Port != 5432 || pgC.Ports[0].ContainerPort != 5432 {
		t.Errorf("postgres Service = %+v", db.Spec)
	}
	env := map[string]corev1.EnvVar{}
	for _, e := range appC.Env {
		env[e.Name] = e
	}
	if env["PGHOST"].Value != db.Name || env["PGPORT"].Value != "5432" || env["PGPASSWORD"].ValueFrom.SecretKeyRef.Name != "postgres" {
		t.Errorf("app database environment = %+v", env)
	}

	deny := get("NetworkPolicy/deny-db").(*networkingv1.NetworkPolicy)
	allow := get("NetworkPolicy/allow-db").(*networkingv1.NetworkPolicy)
	for _, np := range []*networkingv1.NetworkPolicy{deny, allow} {
		if np.Namespace != "sample-controller" || !matches(np.Spec.PodSelector.MatchLabels, pg.Spec.Template.Labels) {
			t.Errorf("%s does not select the Postgres pods", np.Name)
		}
	}
	if len(deny.Spec.Ingress) != 0 || len(deny.Spec.PolicyTypes) != 1 || deny.Spec.PolicyTypes[0] != networkingv1.PolicyTypeIngress {
		t.Errorf("deny-db is not a default-deny for ingress: %+v", deny.Spec)
	}
	in := allow.Spec.Ingress
	if len(in) != 1 || len(in[0].From) != 1 || !matches(in[0].From[0].PodSelector.MatchLabels, app.Spec.Template.Labels) ||
		len(in[0].Ports) != 1 || in[0].Ports[0].Port.IntVal != 5432 {
		t.Errorf("allow-db does not admit only the app on 5432: %+v", in)
	}

	// default-deny ships with the app (config/manager) and blocks the LoadBalancer.
	// allow-lb opens only the app's HTTP port, and only to the LoadBalancer's two sources:
	// world (the Mac, when the pod is on the announcing node) and remote-node (after SNAT).
	appDeny := get("NetworkPolicy/default-deny").(*networkingv1.NetworkPolicy)
	if !matches(appDeny.Spec.PodSelector.MatchLabels, app.Spec.Template.Labels) || len(appDeny.Spec.Ingress) != 0 ||
		len(appDeny.Spec.PolicyTypes) != 1 || appDeny.Spec.PolicyTypes[0] != networkingv1.PolicyTypeIngress {
		t.Errorf("default-deny is not an ingress-only default-deny for the app's pods: %+v", appDeny.Spec)
	}
	if _, ok := objs["NetworkPolicy/allow-lb"]; ok {
		t.Errorf("allow-lb must be a CiliumNetworkPolicy only")
	}
	lb := get("CiliumNetworkPolicy/allow-lb").(*ciliumNetworkPolicy)
	lbIn := lb.Spec.Ingress
	sel := lb.Spec.EndpointSelector
	if lb.Namespace != "sample-controller" || len(sel.MatchExpressions) != 0 || !matches(sel.MatchLabels, app.Spec.Template.Labels) {
		t.Errorf("allow-lb does not select the app's pods in sample-controller: %+v", lb)
	}
	if len(lbIn) != 1 || strings.Join(lbIn[0].FromEntities, ",") != "world,remote-node" ||
		len(lbIn[0].ToPorts) != 1 || len(lbIn[0].ToPorts[0].Ports) != 1 {
		t.Errorf("allow-lb must have one ingress rule, from world and remote-node, on one port: %+v", lbIn)
	} else if p := lbIn[0].ToPorts[0].Ports[0]; p.Port != strconv.Itoa(int(appC.Ports[0].ContainerPort)) ||
		appC.Ports[0].ContainerPort != 8080 || appC.Ports[0].Name != "http" || p.Protocol != string(corev1.ProtocolTCP) {
		t.Errorf("allow-lb must open the app's pod port 8080/TCP (named http), not the Service port: %+v", p)
	}
}
