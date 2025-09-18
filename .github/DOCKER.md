# ProgressDB Docker Quickstart

This file explains how to run the ProgressDB service using the official Docker image published to Docker Hub (image name: `progressdb/progressdb`).

Pull an image

Replace `vX.Y.Z` with the release tag you want (or `latest` if you maintain that tag):

```sh
docker pull docker.io/progressdb/progressdb:v0.1.1
```

Run the container (simple)

```sh
docker run -d \
  --name progressdb \
  -p 8080:8080 \
  -v $PWD/data:/data \
  docker.io/progressdb/progressdb:v0.1.1 --db /data/progressdb
```

What this does

- Exposes the service on `http://localhost:8080`.
- Persists data under the host `./data` directory (mounted into the container at `/data`).
- The example passes `--db /data/progressdb` to set the repository path inside the container.

Docker Compose example

```yaml
version: '3.8'
services:
  progressdb:
    image: docker.io/progressdb/progressdb:v0.1.1
    container_name: progressdb
    ports:
      - "8080:8080"
    volumes:
      - ./data:/data
    command: ["--db", "/data/progressdb"]
```

Useful endpoints

- Admin viewer: `http://localhost:8080/viewer/`
- OpenAPI docs: `http://localhost:8080/docs/`
- Health check: `http://localhost:8080/healthz`
- Prometheus metrics: `http://localhost:8080/metrics`

KMS (optional) — running the KMS as a container

ProgressDB ships a separate KMS container image `progressdb/progressdb-kms`. To run it locally and point the server at it:

```sh
docker run -d \
  --name progressdb-kms \
  -p 6820:6820 \
  -v $PWD/kms-data:/var/lib/progressdb/kms \
  docker.io/progressdb/progressdb-kms:latest --endpoint 0.0.0.0:6820 --data-dir /var/lib/progressdb/kms
```

Then start the server and point it to the KMS:

```sh
docker run -d \
  --name progressdb \
  -p 8080:8080 \
  -e PROGRESSDB_USE_ENCRYPTION=true \
  -e PROGRESSDB_KMS_MODE=external \
  -e PROGRESSDB_KMS_ENDPOINT=host.docker.internal:6820 \
  docker.io/progressdb/progressdb:v0.1.1
```

Environment & configuration

You can configure ProgressDB via CLI flags, environment variables, or a config file. Common env vars:

- `PROGRESSDB_DB_PATH` — default DB path inside the container (example uses `--db /data/progressdb`).
- `PROGRESSDB_API_BACKEND_KEYS` — comma-separated backend/admin keys (for admin SDKs).
- `PROGRESSDB_API_FRONTEND_KEYS` — comma-separated frontend keys for public clients.

Security notes

- Do not expose admin/backend API keys publicly. Use network rules and secure secrets management in production.
- Run the container behind TLS in production (use a reverse proxy or set TLS cert/key in config).

Advanced

- The Docker image is produced by the release pipeline; use the release tag that matches the binary you want.
- For multi-arch images we publish platform manifests so pulling the tag should select the correct architecture automatically.

Debugging

- View logs:
  - `docker logs -f progressdb`
- Run an interactive shell (for debugging only):
  - `docker run --rm -it --entrypoint sh docker.io/progressdb/progressdb:v0.1.1`

Where to get the image

- Docker Hub: https://hub.docker.com/r/progressdb/progressdb
- Releases (binaries & archives): https://github.com/ha-sante/ProgressDB/releases

If you want, the CI can be adjusted to change image tags or push to another registry — open an issue or PR if you need that.
