# container examples

This repository contains examples that show you how to use [`container`](https://github.com/apple/container) and [Containerization](https://github.com/apple/containerization) to build, run, and develop with Linux containers on your Mac.

- [`container`](https://github.com/apple/container) is a tool that you can use to create and run Linux containers as lightweight virtual machines on your Mac. It's written in Swift, and optimized for Apple silicon.
- [Containerization](https://github.com/apple/containerization) is the Swift package that `container` uses for low-level container, image, and process management. You can use it to add Linux container support to your own applications.

## Examples

| Example | Description |
| --- | --- |
| [container-machine-vscode](./container-machine-vscode/) | Develop Linux applications in a container machine with Visual Studio Code and its SSH remote development extension. |
| [container-cluster-gateway](./container-cluster-gateway/) | Run a three-tier web app on a local Kubernetes cluster with `container cluster`, and reach it from your browser through a gateway container. |
| [container-workflows-lab](./container-workflows-lab/) | Take a Swift server app with Postgres through three workflows: verify it on Linux with `container machine`, build and run an image with `container`, and deploy it locally with `container cluster`. |

Each example has its own README with prerequisites and step-by-step instructions.

## Reporting issues

If an example doesn't work as described, open an issue in this repository. If the problem is with `container` or Containerization itself, open an issue in the [container](https://github.com/apple/container/issues) or [Containerization](https://github.com/apple/containerization/issues) repository. 

## Contributing

Contributions are welcome and encouraged! Read the [main contributing guide](https://github.com/apple/containerization/blob/main/CONTRIBUTING.md) to get started.

## Code of conduct

This project follows the [Code of Conduct](https://github.com/apple/.github/blob/main/CODE_OF_CONDUCT.md) used by Container and Containerization.

## Maintainers

See [MAINTAINERS.txt](./MAINTAINERS.txt).

## License

This project is licensed under the [Apache License 2.0](./LICENSE).
