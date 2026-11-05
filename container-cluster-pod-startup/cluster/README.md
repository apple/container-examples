# The example cluster: reference

Reference for the example cluster's Cilium setup. To create the cluster, follow
[step 1 of the README](../README.md#1-create-the-cluster).

The cluster is made with `container cluster create`: one control plane and
three workers, with Cilium as the CNI.
Cilium gives the app's LoadBalancer Service an address on the Mac's default container
network, and it enforces the network policies in `config/`.

| File | What it holds |
| --- | --- |
| `cilium-values.yaml` | The Helm values for Cilium 1.20.1. |
| `cilium.yaml` | Cilium, rendered from `cilium-values.yaml`. `--cni` applies it during create. |
| `lb-pool.yaml` | A `CiliumLoadBalancerIPPool` with one address, `192.168.70.254`. |
| `l2-policy.yaml` | A `CiliumL2AnnouncementPolicy`: answer ARP for that address on `enp0s1`. |

## Why the subnet is pinned

With no subnet set, the default network can get another /24 each time the container
services start. The LoadBalancer address in `lb-pool.yaml`, `192.168.70.254`, is then off
the network, and the Mac cannot reach the app.
Pinning `subnet = "192.168.70.0/24"` in `~/.config/container/config.toml` keeps the network,
and so the address, the same after every restart. The [README](../README.md#before-you-start) has the steps.

## Why Cilium is set up this way

- **Native routing, no tunnel.** Every node is on one L2 segment, the default network,
  so pods route directly between nodes (`routingMode: native`, `autoDirectNodeRoutes`).
  `ipv4NativeRoutingCIDR` is the cluster's pod subnet, `10.244.0.0/16`.
- **Kubernetes IPAM.** kubeadm gives each node a pod CIDR from `10.244.0.0/16`, and Cilium
  uses those (`ipam.mode: kubernetes`). The service subnet is `10.96.0.0/12`.
- **MTU 1280.** The nodes attach to the default network at MTU 1280.
- **kube-proxy stays.** The cluster installs kube-proxy.
  L2 announcements need Cilium's kube-proxy replacement, and both can run together on a
  new cluster.
  With kube-proxy kept, the agent reaches the API server at the `kubernetes` Service
  (`10.96.0.1`), so `k8sServiceHost` is not set: the control-plane IP is not known before
  create.
  `k8sServiceHost: auto` is a Helm `lookup`, and it does not work with `helm template`.
- **`enp0s1`, not `eth0`.** The node NIC is `enp0s1`.
  With `devices: eth0`, every agent exits with
  `unable to determine direct routing device. Use --direct-routing-device to specify it`.
  `cilium-values.yaml` sets both `devices` and `direct-routing-device` to `enp0s1`, and
  `l2-policy.yaml` matches `^enp0s1$`.
- **One pool address, `.254`.** The container network gives containers `.2` to `.253` of
  the /24, in order, and puts released addresses at the end of the queue.
  Nodes, builders and container machines all take addresses from that range.
  `.1` is the gateway on the Mac, so `.254` is the only address on the network that no
  container can get.
- **A higher client rate limit.** Each announced Service holds a `Lease`, which needs more
  API calls than the default 5 QPS (`k8sClientRateLimit` 50/100).
- **No `externalTrafficPolicy: Local`.** L2 announcements do not support it.
  One node, the lease holder, answers ARP for the address and takes the traffic.
  Any node can hold the lease, the control plane too.
  When the pod is on another node, the lease holder forwards the packet and SNATs it to
  its own node IP.
  That is why `config/policy/allow-lb.yaml` admits `remote-node` as well as `world`.

## Render cilium.yaml again

`cilium.yaml` is committed, so a normal run does not need Helm.
After a change to `cilium-values.yaml`, render it again (this needs network access):

```sh
helm repo add cilium https://helm.cilium.io/
helm repo update
helm template cilium cilium/cilium --version 1.20.1 --namespace kube-system \
  -f cluster/cilium-values.yaml > cluster/cilium.yaml
```

Then delete the cluster and create it again ([step 1 of the README](../README.md#1-create-the-cluster)).

## Verify

Optional, about 2 minutes, before you deploy the app.
A throwaway nginx Deployment checks the LoadBalancer address, the L2 announcement,
cross-node traffic and policy enforcement.

```sh
LB=192.168.70.254
```

1. Nodes and agents: 4 nodes `Ready`; `cilium` and `kube-proxy` both 4/4.

   ```sh
   kubectl get nodes; kubectl -n kube-system get ds cilium kube-proxy
   ```

2. Nothing answers on the pool address yet, and the address is on-link (a `bridge`
   interface, no gateway).

   ```sh
   ping -c1 -t1 $LB; route -n get $LB
   ```

3. A LoadBalancer Service gets the address. Three replicas make some traffic cross nodes.

   ```sh
   kubectl create deployment web --image=nginx:1.27-alpine --replicas=3
   kubectl expose deployment web --port 80 --type LoadBalancer
   kubectl get svc web -w             # EXTERNAL-IP 192.168.70.254, then Ctrl-C
   ```

4. The Mac reaches it: six `200` lines.

   ```sh
   for i in 1 2 3 4 5 6; do curl -s -o /dev/null --max-time 5 -w '%{http_code}\n' http://$LB/; done
   ```

5. The Mac's ARP entry for the address is the lease holder's `enp0s1` MAC: the two MACs
   match.

   ```sh
   N=$(kubectl -n kube-system get lease cilium-l2announce-default-web -o jsonpath='{.spec.holderIdentity}'); echo $N
   arp -n $LB
   container exec $N cat /sys/class/net/enp0s1/address
   ```

6. Policy deny: start a client, apply a policy that admits only pods labelled
   `access=true`, and see the request time out.

   ```sh
   kubectl run client --image=curlimages/curl:8.10.1 --command -- sleep 3600
   kubectl wait --for=condition=Ready pod/client --timeout=120s
   kubectl exec client -- curl -s -o /dev/null --max-time 5 -w '%{http_code}\n' http://web   # 200 before
   kubectl apply -f - <<'EOF'
   apiVersion: networking.k8s.io/v1
   kind: NetworkPolicy
   metadata: {name: web-allow-access}
   spec:
     podSelector: {matchLabels: {app: web}}
     ingress: [{from: [{podSelector: {matchLabels: {access: "true"}}}]}]
   EOF
   kubectl exec client -- curl -sS --max-time 5 http://web 2>&1 | grep 'timed out'
   ```

7. Policy allow: label the client, and get a `200` within 5 tries.
   The new identity takes a moment to apply.

   ```sh
   kubectl label pod client access=true
   for i in 1 2 3 4 5; do kubectl exec client -- curl -s -o /dev/null --max-time 5 -w '%{http_code}\n' http://web | grep -q 200 && { echo ALLOWED; break; }; sleep 2; done
   ```

8. Clean up. The app needs the pool's only address, so no other LoadBalancer Service may
   keep it.

   ```sh
   kubectl delete pod client; kubectl delete networkpolicy web-allow-access; kubectl delete svc,deploy web
   kubectl get svc -A | grep LoadBalancer || echo "no LoadBalancer Service"
   ```

## Troubleshooting

| Symptom | Cause and fix |
| --- | --- |
| `create` fails, and the cluster is gone. | When the kube-system pods are not ready in 600 s, create deletes every node and its kubeconfig entry. The usual cause is a slow or failed Cilium image pull. If `HTTP_PROXY` or `HTTPS_PROXY` (or the lowercase names) point at a proxy on `localhost`, unset them: the nodes are VMs and cannot reach the Mac's loopback. Then create again. |
| `get ippools` shows no pool, or the Service has no `EXTERNAL-IP`. | The pool was not applied, or `container network list` shows another subnet than the pool's. Fix `lb-pool.yaml` if the subnet differs, then apply it again: `kubectl apply -f cluster/lb-pool.yaml`. |
| The Service has an `EXTERNAL-IP`, but the Mac gets no answer. | Run verify step 5. `l2-policy.yaml` must match the node NIC, `^enp0s1$`. Check that no VPN, Docker or other hypervisor also uses `192.168.70.0/24` (`netstat -rn \| grep 192.168.70`); if one does, pick another subnet. |
| Some requests fail, only when the pod is on another node. | Native routing between the nodes does not work. Delete the three routing lines in `cilium-values.yaml` (`routingMode`, `ipv4NativeRoutingCIDR`, `autoDirectNodeRoutes`) to use VXLAN, render again, and create the cluster again. |
| An L7 (HTTP) policy does not work. | This setup was tested with L3/L4 policy only. |
