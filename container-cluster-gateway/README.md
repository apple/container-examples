# Example: Run a three-tier app on a container cluster behind a gateway

This example shows you how to use `container cluster` to run a small three-tier web application on a local Kubernetes cluster, and how to reach it from your Mac's browser by name through a gateway container.

The app is a shared notepad. Anyone who can reach the page can post a short note with their name. Notes are stored in [Valkey](https://valkey.io/) and served through a [Flask](https://flask.palletsprojects.com/) API, with a [React](https://react.dev/) frontend built with [Vite](https://vitejs.dev/) and served by [nginx](https://nginx.org/). The frontend polls for new notes every few seconds, so multiple browser windows stay in sync.

## Prerequisites

Install and start before running the demo:

- Apple `container` release 1.7.0 or later
- [`kubectl`](https://kubernetes.io/docs/tasks/tools/)
- The `test` DNS domain, configured as the default domain and registered with macOS, as described in [Set up DNS-based container names](../../docs/networking.md#set-up-dns-based-container-names)
- [Helm](https://helm.sh/)

## Architecture

```
  ┌──────────┐
  │  Browser │   http://notepad.gateway.test
  └────┬─────┘
       │
  ┌────┴───────────────────────────┐
  │  gateway    nginx container    │   network alias .gateway
  └────┬───────────────────────────┘
       │  NodePort 31080 on the worker nodes
  ┌────┴───────────────────────────┐
  │  Traefik    Gateway API        │   routes by Host header
  └────┬───────────────────────────┘
       │
   HTTPRoutes ──► web, api Services
                      │
                      └─► api ──► valkey
```

A cluster's nodes share one container network, and the pods reach each other through Kubernetes Services. The gateway is an nginx container on that same network. It is started with a network alias that has a leading dot:

```console
% container run -d --name gateway --network <nodes-network>,alias=.gateway ...
```

The alias makes every name below `gateway.test` resolve to the gateway, so `notepad.gateway.test` and `api.gateway.test` both reach it. The gateway forwards requests to Traefik on the worker nodes. Traefik implements the Kubernetes [Gateway API](https://gateway-api.sigs.k8s.io/): in `gateway-api/routes.yaml`, a Gateway listens for names below `gateway.test`, and HTTPRoutes use the `Host` header to choose between the `web` and `api` Services. Adding another name takes only another HTTPRoute, with no change to DNS.

## Set up

Follow these steps to start the cluster and gateway, and build and deploy the application.

### 1. Build the images

```bash
container build -t demo-web:latest web
container build -t demo-api:latest api
```

### 2. Create the cluster

```bash
container cluster create --workers 2 --shared-image-store
```

This pulls the Kubernetes node image on first use, starts a control plane node and two worker nodes, waits for the nodes to be Ready, and merges the cluster into `~/.kube/config`. The default `cluster-dev` cluster gets a kubectl context of the same name. To use another name, pass `--name` to `container cluster create` and `--context` or the cluster name to the scripts below.

The `--shared-image-store` option allows the cluster to pull images from the local image store that `container` manages, without the need to push them to a registry or load them manually into cluster nodes.

### 3. Apply the manifests

```bash
kubectl --context cluster-dev apply -f deployment
```

### 4. Create the gateway

```bash
./scripts/deploy-gateway.sh
```

The script installs the Gateway API CRDs and Traefik, applies the Gateway and HTTPRoutes, finds the container network of the worker nodes, writes the gateway's nginx configuration under `.gateway/`, and starts the `gateway` container. When it finishes, it prints the app's address.

The script reads three environment variables when its defaults are wrong:

- `GATEWAY_NETWORK` is the container network that the cluster nodes are on. The default is found from the address of a worker node.
- `GATEWAY_API_VERSION` is the Gateway API release whose standard CRDs are installed. The default is `v1.7.0`.
- `TRAEFIK_CHART_VERSION` pins the Traefik Helm chart. The default is the latest.

## Examine what was deployed

Use the `container` CLI to see the containers and network behind the cluster:

```console
# The cluster
container cluster list

# The control plane and worker nodes, with their addresses
# (container list does not show cluster nodes)
container cluster list --nodes

# The gateway container
container list

# The network that the nodes and the gateway share
container network list

# The gateway's addresses and its .gateway alias
container inspect gateway | jq '.[0].status.networks'

# The images that were built
container image list | grep demo-
```

Use `kubectl` to see what runs inside the cluster:

```console
# The nodes, with their container network addresses
kubectl --context cluster-dev get nodes -o wide

# The app's pods and which worker node each runs on
kubectl --context cluster-dev get pods -o wide

# The Services that connect web, api and valkey
kubectl --context cluster-dev get services

# The Gateway and the HTTPRoutes that route notepad.gateway.test and api.gateway.test
kubectl --context cluster-dev get gateway,httproute

# Traefik, which implements the Gateway API
kubectl --context cluster-dev get pods --namespace traefik
```

## Try the application and API

1. Open `http://notepad.gateway.test` in a browser.
2. Duplicate the tab into a new window and arrange the two windows side by side.
3. Enter a name and post a note in the first window, then post a note from the second window. Each note appears in both windows within a few seconds.
4. Look at the notes on the API server: open `http://api.gateway.test/api/notes` in the browser, or run:

   ```console
   curl -s http://api.gateway.test/api/notes | jq
   ```

## Clean up

Remove the gateway, the routes and Traefik:

```console
./scripts/remove-gateway.sh
```

Stop and delete the cluster, including its worker nodes:

```console
container cluster delete --name cluster-dev
```
