// rbac_gate_test.go asserts one property: every +kubebuilder:rbac marker in this package
// is granted by the hand-written ClusterRole in config/rbac/role.yaml, and nothing more.
package controller

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/yaml"
)

var markerRE = regexp.MustCompile(`\+kubebuilder:rbac:groups="?([^",]*)"?,resources=([^,]+),verbs=(\S+)`)

func TestRBACMarkersMatchClusterRole(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "config", "rbac", "role.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var role rbacv1.ClusterRole
	if err := yaml.UnmarshalStrict(b, &role); err != nil {
		t.Fatal(err)
	}
	var granted, marked []string
	for _, r := range role.Rules {
		for _, g := range r.APIGroups {
			for _, res := range r.Resources {
				for _, v := range r.Verbs {
					granted = append(granted, g+"/"+res+"/"+v)
				}
			}
		}
	}
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, _ := os.ReadFile(f)
		for _, m := range markerRE.FindAllStringSubmatch(string(src), -1) {
			for _, res := range strings.Split(m[2], ";") {
				for _, v := range strings.Split(m[3], ";") {
					marked = append(marked, m[1]+"/"+res+"/"+v)
				}
			}
		}
	}
	sort.Strings(granted)
	sort.Strings(marked)
	if len(marked) == 0 || strings.Join(granted, " ") != strings.Join(marked, " ") {
		t.Fatalf("role grants %v, markers ask for %v", granted, marked)
	}
}
