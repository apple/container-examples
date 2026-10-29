# swift-dev.Dockerfile  
FROM swift:6.3-noble

ENV DEBIAN_FRONTEND=noninteractive

# systemd provides /sbin/init, which is what makes the image bootable as a machine.
# sudo lets your user install packages later; curl is handy for testing.
RUN apt-get -qq update && apt-get -qq install -y \
        systemd systemd-sysv sudo curl \
    && rm -rf /var/lib/apt/lists/*

CMD ["/sbin/init"]