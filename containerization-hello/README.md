# Example: Launch a Linux container from Swift with Containerization

This example shows you how to use the [Containerization](https://github.com/apple/containerization) Swift package to run a simple Linux container in about 50 lines of Swift. It fetches a Linux kernel, creates a container from an Alpine image, and starts an interactive shell in it.

## Prerequisites

Install before running the demo:

- A Mac with Apple silicon, running macOS 26 or later
- Xcode, or a Swift toolchain that supports Swift 6.2

You don't need `container` installed. The example pulls the images it needs from the registry into its own image store.

> [!IMPORTANT]
> A bug in the `vmnet` framework on macOS 26 causes network creation to fail for applications located under your `Documents` or `Desktop` directories. Clone this repository somewhere else, such as `~/projects`.

## How it works

The code is in [`Sources/ctr-hello/main.swift`](./Sources/ctr-hello/main.swift). The example creates a `ContainerManager`, uses it to create a container, then drives the container through its lifecycle with `create()`, `start()`, `wait()`, and `stop()`.

Everything the example downloads lives in the `.state` directory, which is gitignored:

| Path | Contents |
| --- | --- |
| `.state/vmlinux-arm64` | The Linux kernel, downloaded the first time you run `make` and verified against a SHA-256 digest. |
| `.state/image-store` | The init image and the Alpine image, pulled from their registries, and the container's root filesystem. |

## Build and run

From the `containerization-hello` directory, build and run the example:

```console
% make
```

The first run downloads the kernel, then builds and runs the example. After the build completes, you should see output like this:

```console
Starting container example...
Fetching container initial filesystem...
Creating container from docker.io/library/alpine:3.16...
Starting container...
/ #
```

You've started a container from Swift. Type `exit` to stop it.

If you already have `container` installed and want to use the kernel that it installed, copy that kernel into place before you run `make`:

```console
% mkdir -p .state
% cp "$(ls -t ~/Library/Application\ Support/com.apple.container/kernels/vmlinux-* | head -1)" .state/vmlinux-arm64
```

### Troubleshooting

If you get `error: compiled module was created by a newer version of the compiler`, try building from the default macOS terminal.

If you get the following error, move the `ctr-hello` binary to `/var/tmp` and run it from there:

```console
Fatal error: Error raised at top level: unsupported: "failed to create vmnet network with status vmnet_return_t(rawValue: 1001)"
```

## Modify the example

Try changing the example and running `make` again:

- Change the command that the container runs.
- Change the image, for example to a different Alpine version or another distribution.

## Clean up

Delete the downloaded kernel and images:

```console
% make clean-state
```

Delete the build products:

```console
% make clean
```
