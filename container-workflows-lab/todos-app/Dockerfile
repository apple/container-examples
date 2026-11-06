# User with a builder image.
FROM swift:6.3-noble AS builder

# Set up an area for the code and the build.
WORKDIR /code

# First resolve dependencies, this gets cached in a layer.
COPY ./Package.* ./
RUN swift package resolve \
        $([ -f ./Package.resolved ] && echo "--force-resolved-versions" || true)

# Copy the Sources and Public dirs.
COPY ./Sources ./Sources
# TODO: currently public not in here
# COPY ./Public ./Public

# Build the application in release mode with a statically linked runtime.
# the product here is tied to the target name in Package.swift
RUN swift build -c release \
        --product SwiftServerTodos \
        --static-swift-stdlib

# Switch over to a lighter-weight runtime image.
FROM swift:6.3-noble-slim

# The slim image already includes ca-certificates and tzdata, so no apt step is needed.

# Create a directory for the server binary and resources.
RUN mkdir -p /app/bin
RUN mkdir -p /app/lib

# Copy over the binary and the Public directory.
COPY --from=builder /code/.build/release/SwiftServerTodos /app/bin/
# TODO: currently public not in here
# COPY --from=builder /code/Public /app/Public

COPY --from=builder /usr/libexec/swift/linux/swift-backtrace-static /app/lib/

# Switch over to the working directory.
WORKDIR /app

# Expose the default port.
EXPOSE 8080

# Provide configuration needed by the built-in crash reporter and some sensible default behaviors.
ENV SWIFT_BACKTRACE=enable=yes,sanitize=yes,threads=all,images=all,interactive=no,swift-backtrace=/app/lib/swift-backtrace-static

# Start the server.
CMD /app/bin/SwiftServerTodos serve --hostname 0.0.0.0 --port 8080
