# Container Workflows Lab

In this lab you take a Swift server app (HTTP API + Postgres) through these Apple Container workflows:

| Stage | Tool | What it shows |
|---|---|---|
| 1. Develop | your mac! | The app builds and runs on your Mac |
| 2. Verify on Linux | **container machine** | A Linux VM that sees your Mac files *at the same path*, so you can edit on the Mac and rebuild on Linux instantly |
| 3. Containerize | **container** | Build and run an OCI image of the app |
| 4. Deploy locally | **container cluster** | Run the app + Postgres on a local cluster before it goes to staging |

---

## Prerequisites

| Requirement | Version | Get it |
|---|---|---|
| Apple silicon Mac running **macOS 26** | macOS 26 | |
| [Apple Container](https://github.com/apple/container) | **1.6.1 or later** | [Releases](https://github.com/apple/container/releases) |
| Swift toolchain on macOS | **6.3 or newer** | [Xcode](https://developer.apple.com/xcode/) or [swift.org](https://www.swift.org/install/macos/) |
| `kubectl` | **1.37 or newer** | [Install guide](https://kubernetes.io/docs/tasks/tools/) (or `brew install kubectl`) |

- The container system service running:
  ```bash
  container system start
  container system status
  ```
- Host port `8080` free (used in Parts 3 and 4).

### Pre-pull the images (recommended)

Run these before the workshop so the large downloads are already cached:

```bash
container image pull --platform linux/arm64 swift:6.3-noble
container image pull --platform linux/arm64 swift:6.3-noble-slim
container image pull --platform linux/arm64 postgres:17-alpine
container image pull --platform linux/arm64 docker.io/kindest/node:v1.35.5@sha256:ce977ae6d65918d0b58a5f8b5e940429c2ce42fa3a5619ec2bbc60b949c0ac95
```

### Pre-flight check

Run these and confirm the expected result for each:

```bash
sw_vers -productVersion        # 26 or later
uname -m                       # arm64
swift --version                # Swift version 6.3 or newer
container --version            # container CLI version 1.6.1 or later
container system status        # running
kubectl version --client       # prints a client version
lsof -i :8080                  # prints nothing (port is free)
container image list           # shows the images above
```

---

## Repo layout

```
container-workflows-lab/
├── README.md                  # this guide
├── todos-app/                 # Swift server todos app (from swiftlang/swift-server-todos-tutorial, Completed/)
│   ├── UPSTREAM.md            # upstream source, commit, and lab modifications
│   ├── Package.swift
│   ├── Dockerfile             # app image (used in Part 3)
│   └── Sources/SwiftServerTodos/
│       ├── ETagMiddleware.swift   # ← the feature you'll be verifying
│       └── ServerService.swift
├── machine/
│   └── swift-dev.Dockerfile   # image for the Container Machine
└── cluster/
    └── todos-cluster.yaml     # Postgres + todos API manifests
```

---

## The scenario

You added a feature to the todos API: an **ETag middleware** that hashes each response body and returns it in an `ETag` header, so clients can cache responses. Take a look at `todos-app/Sources/SwiftServerTodos/ETagMiddleware.swift`.

Things to notice in `ETagMiddleware.swift`:

- `import CryptoKit`: Apple's crypto framework, used for `SHA256.hash(...)`.
- `response.body.collect(...)`: OpenAPI handlers stream their bodies, so the middleware collects the bytes before hashing.

In `ServerService.swift`, enable this middleware by uncommenting the `ETagMiddleware` line:

```swift
app.middleware.use(RequestLoggerInjectionMiddleware())
app.middleware.use(TracingMiddleware())
// Enable ETagMiddleware
app.middleware.use(ETagMiddleware())        
app.traceAutoPropagation = true
```

## Part 1 — Build on macOS

## Build it

From the `container-workflows-lab` directory:

`[macOS]`
```bash
cd todos-app
swift build
```

**Expected:** `Build complete!`

> While building, look around the files: APIHandler (basic CRUD), ETagMiddleware (hashing the response body). Swift build reads Package.swift to read what the package contains and resolves their dependencies.


**Recap:** The feature builds fine on macOS. Production runs on Linux, so let's better check that.

---

## Part 2 — Verify on Linux with Container Machine

### 2.1 Build the machine image

The machine image is the official `swift:6.3-noble` image (the same toolchain as the app's own `Dockerfile`) plus `systemd`, which makes it bootable as a machine.

Go back up to `container-workflows-lab`:

`[macOS]`
```bash
cd ..
container build -f machine/swift-dev.Dockerfile -t swift-dev:lab machine/
```

If you didn't pre-pull the image, this downloads the Swift image (around 1 GB) and takes some time.

The `swift:6.3-noble` image is essentially Ubuntu 24.04 with the Linux Swift 6.3 compiler installed.

### 2.2 Create the machine

`[macOS]`
```bash
container machine create swift-dev:lab --name swift-dev
container machine list
```

Open a shell in the machine and look around. The shell starts in the same directory as your Mac shell (`container-workflows-lab`).

Should say Linux instead of Darwin
```bash
container machine run -n swift-dev
uname 
```

Expected: `Swift version 6.3.x (swift-6.3.x-RELEASE)`
```bash
swift --version
```

Mirrors username and current working directory from your Mac
```bash
whoami
pwd
```

### 2.3 Build on Linux

In the machine shell, build the Linux artifacts.

`[swift-dev]`
```bash
cd todos-app  
swift build --scratch-path ~/.cache/todos-build
```

> **Why `--scratch-path`?** It keeps Linux build output on the machine's own disk, separate from the macOS `.build/` folder on your Mac. The two platforms' build artifacts never collide.

**Expected failure:**
```
error: no such module 'CryptoKit'
```

**Why?** CryptoKit is an Apple-platform framework and doesn't exist on Linux. The code compiled on macOS, but it would have failed in CI or production. You found this in a couple of minutes, on your laptop.

### 2.4 Fix it on your Mac

Make these edits in your editor on the Mac. Leave the machine shell open, because you'll use it again in 2.5.

After doing some research, you find out that the cross-platform fix is **[swift-crypto](https://github.com/apple/swift-crypto)**, Apple's open-source implementation of the CryptoKit API that works on every platform. Its `Crypto` module has the same API, so you need to change the import and add the dependency.

In `todos-app/Package.swift`, uncomment to add the dependency to the package and the executable target.

```swift
// Crypto 
.package(url: "https://github.com/apple/swift-crypto.git", from: "3.0.0"),
```

```swift
// Crypto
.product(name: "Crypto", package: "swift-crypto"),
```

Then modify the import in `todos-app/Sources/SwiftServerTodos/ETagMiddleware.swift`. 

```diff
- import CryptoKit
+ import Crypto
```

### 2.5 Rebuild on Linux

Switch back to your machine shell. You don't need to copy or sync anything, because the machine sees your edit at the same path.

`[swift-dev]`
```bash
swift build --scratch-path ~/.cache/todos-build
```

**Expected:** `Build complete!`

**Recap:** You used Container Machine to build your app on a Linux environment. You found that you're using a platform-specific API. Then you edited on macOS with your editor and verified on Linux right away, and Container Machine never needed a copy or a sync step. You caught a platform bug during development instead of in CI.

---

## Part 3 — Containerize the app

### 3.1 Build the app image

The app ships with a multi-stage `Dockerfile`: a `swift:6.3-noble` builder stage compiles the app on Linux, and a slim `swift:6.3-noble-slim` runtime stage runs it. Because you fixed the Linux build in Part 2, this now succeeds.

Exit the machine shell to return to your Mac shell, which is still in `container-workflows-lab`:

`[swift-dev]`
```bash
exit
```

Then build the image:

`[macOS]`
```bash
cd todos-app
container build -t todos:lab .
container image list | grep todos
```

This will take some time. 

### 3.2 Run the app as a container

The app needs a database, so run Postgres as its own container first.

`[macOS]`
```bash
container run -d --name todos-db -e POSTGRES_HOST_AUTH_METHOD=trust postgres:17-alpine
container list
```

> `POSTGRES_HOST_AUTH_METHOD=trust` is for this local lab only.

Every container gets its own IP address. Find the `todos-db` address in the `ADDR` column of `container list` (it looks like `192.168.64.x`)

Now start the app and point it at the database. Replace `<DB-IP>` with the address you just found:

`[macOS]`
```bash
container run -d --name todos-app -p 8080:8080 \
  -e POSTGRES_URL="postgres://postgres@<DB-IP>:5432/postgres?sslmode=disable" \
  todos:lab
```

- `-d` runs the container in the background.
- `-p 8080:8080` publishes the app's port to your Mac as `localhost:8080`.
- `POSTGRES_URL` tells the app where its database is. The app reaches Postgres directly over the container network.

> **Tip:** Instead of looking up the database IP, you can use `container system dns` to create a local DNS domain (for example, `sudo container system dns create test`).

Check that it's up, then call the API from your Mac:

`[macOS]`
```bash
container logs todos-app
curl -i http://localhost:8080/api/todos
```

**Expected:** `HTTP/1.1 200 OK` with an `etag` header, which comes from the middleware you fixed and verified in Part 2.

**Recap:** You built the app for Linux and verified that it can be run as a container!

### 3.3 Clean up before Kubernetes

Stop and remove both containers. Part 4 reuses port `8080` and the `todos-app` name, so skipping this causes conflicts later.

`[macOS]`
```bash
container stop todos-app todos-db
container delete todos-app todos-db
```
**Recap:** We verified this feature works on the Linux environment, so we went ahead and created a container image. Then we spun up two containers: one app container using this image, and another using postgres to confirm that the app is working.

---

## Part 4 — Run on local Kubernetes

### 4.1 Create the cluster

Downloading the Kubernetes node image (kindest/node 343 MB) the first time, then starting the cluster. This command creates a single node Kubernetes cluster.

`[macOS]`
```bash
container cluster create --name todos-cluster
container cluster list
```

`[cluster]`
```bash
kubectl get nodes
```

**Expected:** one node in `Ready` state.

### 4.2 Load images into the cluster

The cluster can't see your local image store, so load the images explicitly. `todos:lab` was never pushed to a registry.

`[macOS]`
```bash
container image pull --platform linux/arm64 postgres:17-alpine
container cluster load-image --name todos-cluster postgres:17-alpine
container cluster load-image --name todos-cluster todos:lab
```

### 4.3 Review the manifests

Go back up to `container-workflows-lab`:

`[macOS]`
```bash
cd ..
cat cluster/todos-cluster.yaml
```

What's in it:

| Resource | Purpose |
|---|---|
| `PersistentVolume` / `PersistentVolumeClaim` `postgres-data` | 1 Gi `hostPath` volume on the node so data survives pod restarts |
| `Deployment` `postgres` | Postgres 17 (Alpine), `Recreate` strategy (never two pods on one volume), `pg_isready` readiness probe |
| `Service` `postgres` | Cluster DNS name `postgres:5432` for the app |
| `Deployment` `todos` | The `todos:lab` image with `imagePullPolicy: Never` (use the loaded image, never a registry) and a `/health` readiness probe |
| `Service` `todos` | Port 80 → container port 8080 |

Things to notice:

- `PGDATA=/var/lib/postgresql/data/pgdata` puts the data directory in a subfolder of the mount. Postgres refuses to initialize directly in a mount point's root.
- `POSTGRES_HOST_AUTH_METHOD=trust` is for this local lab only. Never use it in a real deployment.
- The app finds the database through `POSTGRES_URL=postgres://postgres@postgres:5432/...`, where `postgres` is the Service name.

### 4.4 Deploy

`[cluster]`
```bash
kubectl apply -f cluster/todos-cluster.yaml
kubectl get pods -w
```

### 4.5 Test the app

Port-forward in **a second terminal** (it keeps running):

`[cluster]`
```bash
kubectl port-forward service/todos 8080:80
```

Back in your first terminal, create a todo and read it back:

`[macOS]`
```bash
curl -X POST http://localhost:8080/api/todos \
  -H 'Content-Type: application/json' \
  -d '{"contents": "hello from apple container"}'

curl -i http://localhost:8080/api/todos
```

**Expected:** the response includes the `etag` header from the middleware you verified in Part 2:
```
HTTP/1.1 200 OK
etag: "113937eb7944717f87f153e1377a9917f6d53d0a444956c08840afc5f961bxxx"
...
      "contents" : "hello from apple container",
```

**Recap:** The app is running on Kubernetes, built from code you verified on Linux and written on macOS.

---

## Part 5 — Explore the cluster

### Prove the data persists

Delete the Postgres pod. The Deployment recreates it on the same volume.

`[cluster]`
```bash
kubectl delete pod -l app=postgres
kubectl get pods -w          # wait for the new postgres pod to be 1/1 Running, then Ctrl+C
```

If the port-forward dropped, restart it in the second terminal, then:

`[macOS]`
```bash
curl http://localhost:8080/api/todos
```

**Expected:** `hello from apple container` is still there.

### Look around

`[cluster]`
```bash
kubectl get all                                  # deployments, replicasets, pods, services
kubectl describe deployment todos                # probes, image, events
kubectl logs deploy/todos                        # app logs
kubectl get pv,pvc                               # storage binding
kubectl scale deployment todos --replicas=3      # scale the API (Postgres stays at 1)
kubectl get pods -l app=todos -o wide
kubectl rollout restart deployment todos         # rolling restart
kubectl rollout status deployment todos
```

---

## Part 6 — Teardown

Stop the port-forward (Ctrl+C in the second terminal), then, back in your first terminal (still in `container-workflows-lab`):

`[cluster]`
```bash
kubectl delete -f cluster/todos-cluster.yaml
```

`[macOS]`
```bash
container cluster delete --name todos-cluster
container machine delete swift-dev
container image delete todos:lab swift-dev:lab
container image delete swift:6.3-noble swift:6.3-noble-slim postgres:17-alpine
```
