FROM debian:bookworm-slim

RUN groupadd --gid 1000 progressdb && \
    useradd --uid 1000 --gid 1000 --create-home --home-dir /home/progressdb progressdb

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl && \
    rm -rf /var/lib/apt/lists/*

# The binaries `progressdb` and `prgcli` will be provided by GoReleaser in the build context root.
COPY progressdb /usr/local/bin/progressdb
COPY prgcli /usr/local/bin/prgcli
RUN chmod +x /usr/local/bin/progressdb /usr/local/bin/prgcli

USER progressdb
WORKDIR /home/progressdb

VOLUME ["/data"]
EXPOSE 8080

HEALTHCHECK --interval=15s --timeout=3s --start-period=10s \
  CMD curl -fsS http://127.0.0.1:8080/healthz || exit 1

ENTRYPOINT ["/usr/local/bin/progressdb"]
CMD ["--config", "/data/config.yaml"]

