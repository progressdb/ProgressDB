FROM debian:bookworm-slim

RUN groupadd --gid 1000 prgkms && \
    useradd --uid 1000 --gid 1000 --create-home --home-dir /home/prgkms prgkms

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl && \
    rm -rf /var/lib/apt/lists/*

# The binary `prgkms` will be provided by GoReleaser in the build context root.
COPY prgkms /usr/local/bin/prgkms
RUN chmod +x /usr/local/bin/prgkms

USER prgkms
WORKDIR /home/prgkms

VOLUME ["/data"]
EXPOSE 6820

HEALTHCHECK --interval=15s --timeout=3s --start-period=10s \
  CMD curl -fsS http://127.0.0.1:6820/healthz || exit 1

ENTRYPOINT ["/usr/local/bin/prgkms"]