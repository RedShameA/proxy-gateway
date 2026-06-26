# --- Frontend ---
FROM node:24-alpine AS web-build
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build && mkdir -p /out/dist && cp -a dist/. /out/dist/

# --- Backend ---
FROM golang:1.26-alpine AS build
ARG VERSION=dev
ARG VCS_REF=unknown
ARG VCS_URL=https://github.com/RedShameA/proxy-gateway
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -tags "with_quic with_wireguard with_grpc with_utls" -ldflags="-s -w -X main.version=${VERSION} -X main.revision=${VCS_REF} -X main.source=${VCS_URL}" -o /out/proxygateway ./cmd/proxygateway

# --- Final image ---
FROM alpine:3.22
ARG VERSION=dev
ARG VCS_REF=unknown
ARG VCS_URL=https://github.com/RedShameA/proxy-gateway
LABEL org.opencontainers.image.source=$VCS_URL \
	org.opencontainers.image.version=$VERSION \
	org.opencontainers.image.revision=$VCS_REF \
	org.opencontainers.image.licenses=AGPL-3.0-or-later
RUN apk add --no-cache ca-certificates su-exec \
	&& addgroup -S app \
	&& adduser -S -G app app \
	&& mkdir -p /data && chown app:app /data

COPY --from=build /out/proxygateway /usr/local/bin/proxygateway
COPY --from=web-build /out/dist /app/web/dist
COPY LICENSE /usr/share/licenses/proxy-gateway/LICENSE
COPY --chmod=755 docker/entrypoint.sh /usr/local/bin/entrypoint.sh

EXPOSE 8080
VOLUME ["/data"]

ENTRYPOINT ["entrypoint.sh"]
CMD ["proxygateway"]
