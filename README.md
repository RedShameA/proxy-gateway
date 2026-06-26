# Proxy Gateway

Proxy Gateway 是一个单端口代理管理服务。它通过单一容器端口提供 Web 控制台、管理 API、HTTP 代理和 SOCKS5 代理，便于集中管理节点、访问策略和代理凭证。

[English README](README.en.md)

## 项目特性

- 单端口提供 Web 控制台、管理 API、HTTP Proxy 和 SOCKS5 Proxy。
- 支持订阅导入和手动导入节点。
- 支持固定节点、最快节点、随机节点和双跳链路策略。
- 使用 SQLite 持久化，数据默认存放在 `/data`。
- 内置 GeoIP 数据库更新，用于出口国家识别和过滤。

## 快速开始

```bash
docker run -d \
  --name proxy-gateway \
  -p 8080:8080 \
  -v proxy-gateway-data:/data \
  ghcr.io/redshamea/proxy-gateway:latest
```

打开 `http://localhost:8080`。首次启动时，如果还没有管理员密码，系统会引导完成初始化。

## 使用方式

1. 在 Web 控制台中导入或创建节点。
2. 创建访问策略和代理凭证。
3. 在代理客户端中使用访问策略标识和凭证密码连接 HTTP Proxy 或 SOCKS5 Proxy。
4. 在控制台中查看节点状态、日志和维护记录。

## 获取镜像

镜像发布在 GitHub Container Registry（GHCR）：

```bash
docker pull ghcr.io/redshamea/proxy-gateway:latest
```

生产环境建议挂载持久化卷，并保持 `/data` 目录持久化。

每次发布都会同时推送 `latest`、`vMAJOR.MINOR.PATCH` 和 `sha-<commit>` 标签。生产环境建议优先使用版本标签，例如 `v0.1.0`；需要固定到明确源码版本时，可以使用 `sha-<commit>` 标签。镜像包含 `org.opencontainers.image.version` 和 `org.opencontainers.image.revision` 标签，运行中的服务也会在 `/api/system/setup-status` 返回对应版本和 revision，可用于追溯到公开源码 commit。

## 文档

- [术语表](docs/glossary.md)
- [产品设计](docs/product-design.md)

## 许可证

Proxy Gateway 使用 [GNU Affero General Public License v3.0 or later](LICENSE) 发布。

## 本地开发

```bash
go test ./...
npm test --prefix web
npm run typecheck --prefix web
docker build -t proxygateway:local .
```
