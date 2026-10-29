# Upstream source

This directory is a copy of the `Completed/` directory from
**[swiftlang/swift-server-todos-tutorial](https://github.com/swiftlang/swift-server-todos-tutorial)**.

## Intentional bug: please don't fix it

On the `main` branch, `ETagMiddleware.swift` uses `import CryptoKit` **on purpose**.
CryptoKit isn't available on Linux, so the Linux build in Part 2 of the lab fails.
Fixing that failure is the point of the lab. 