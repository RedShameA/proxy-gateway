# Proxy Gateway

Proxy Gateway is a single-port proxy management service. It exposes the web console, management API, HTTP proxy, and SOCKS5 proxy through one container port, making it easy to manage nodes, access profiles, and proxy credentials in one place.

[中文 README](README.md)

## Project Features

- One port for the web console, management API, HTTP Proxy, and SOCKS5 Proxy.
- Subscription import and manual node import.
- Fixed, fastest, random, and two-hop chain proxy selection.
- SQLite persistence under `/data`.
- Built-in GeoIP database updates for egress-country detection and filtering.

## Quick Start

```bash
docker run -d \
  --name proxy-gateway \
  -p 8080:8080 \
  -v proxy-gateway-data:/data \
  ghcr.io/redshamea/proxy-gateway:latest
```

Open `http://localhost:8080`. On first startup, if no admin password exists yet, the service will guide you through initialization.

## Usage

1. Import or create nodes in the web console.
2. Create access profiles and proxy credentials.
3. Use the access profile identifier and credential password in an HTTP Proxy or SOCKS5 Proxy client.
4. Review node status, logs, and maintenance records in the console.

## Docker Image

The image is published on GitHub Container Registry (GHCR):

```bash
docker pull ghcr.io/redshamea/proxy-gateway:latest
```

For production use, mount `/data` on a persistent volume.

Each release publishes `latest`, `vMAJOR.MINOR.PATCH`, and `sha-<commit>` tags. For production use, prefer a version tag such as `v0.1.0`; use the `sha-<commit>` tag when you need to pin an image to an exact source revision. Images include `org.opencontainers.image.version` and `org.opencontainers.image.revision` labels, and the running service also returns the version and revision from `/api/system/setup-status` for tracing back to the corresponding public source commit.

## Logs

Backend process logs are written to stdout/stderr, so they are available from `docker logs` or the binary's console. The default log level is `info`; set `PROXYGATEWAY_LOG_LEVEL` to `debug`, `info`, `warn`, or `error` to change it:

```bash
docker run -d \
  --name proxy-gateway \
  -p 8080:8080 \
  -v proxy-gateway-data:/data \
  -e PROXYGATEWAY_LOG_LEVEL=debug \
  ghcr.io/redshamea/proxy-gateway:latest
```

For a local binary:

```bash
PROXYGATEWAY_LOG_LEVEL=warn ./proxygateway
```

## Documentation

- [Glossary](docs/glossary.md)
- [Product Design](docs/product-design.md)

## License

Proxy Gateway is released under the [GNU Affero General Public License v3.0 or later](LICENSE).

## Local Development

```bash
go test ./...
npm test --prefix web
npm run typecheck --prefix web
docker build -t proxygateway:local .
```
