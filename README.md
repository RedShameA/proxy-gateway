# Proxy Gateway

Proxy Gateway 是一个单端口代理管理服务。它通过单一容器端口提供 Web 控制台、管理 API、HTTP 代理和 SOCKS5 代理，便于集中管理节点、访问策略和代理凭证。

[English README](README.en.md)

## 项目特性

- 单端口提供 Web 控制台、管理 API、HTTP Proxy 和 SOCKS5 Proxy。
- 支持订阅导入和手动导入节点。
- 支持固定节点、最快节点、随机节点和双跳链路策略。
- 默认使用 SQLite 持久化，数据存放在 `/data`；也支持 PostgreSQL。
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

## 数据库

默认不需要配置数据库环境变量，服务会使用 `/data/proxygateway.db` 作为 SQLite 数据库。`PROXYGATEWAY_DB_DRIVER` 可设置为 `sqlite` 或 `postgres`；`postgresql` 仅作为兼容别名接受，文档和示例统一使用 `postgres`。如果配置了 `PROXYGATEWAY_DB_DSN`，必须同时显式设置 `PROXYGATEWAY_DB_DRIVER`。

PostgreSQL 示例：

```bash
docker run -d \
  --name proxy-gateway \
  -p 8080:8080 \
  -v proxy-gateway-data:/data \
  -e PROXYGATEWAY_DB_DRIVER=postgres \
  -e 'PROXYGATEWAY_DB_DSN=postgres://proxygateway:password@postgres.example:5432/proxygateway?sslmode=require' \
  ghcr.io/redshamea/proxy-gateway:latest
```

本地二进制连接本机 PostgreSQL：

```bash
PROXYGATEWAY_DB_DRIVER=postgres \
PROXYGATEWAY_DB_DSN='postgres://proxygateway:proxygateway@127.0.0.1:5432/proxygateway?sslmode=disable' \
./proxygateway
```

PostgreSQL 需要 14+。database/schema 需要提前创建，并且应由 Proxy Gateway 独占使用，包括 `goose_db_version` 在内不应属于其他应用；应用负责建表和迁移，不执行 `CREATE DATABASE`。本地测试可使用 `sslmode=disable`，生产环境应按数据库服务要求配置 `sslmode`。即使使用 PostgreSQL，仍建议挂载并持久化 `/data`，因为 GeoIP 等本地数据文件仍会写入该目录。本版本不提供 SQLite 到 PostgreSQL 的自动迁移工具，已有 SQLite 用户切换前需要自行迁移数据或使用空库。

## 日志

后端进程日志默认输出到 stdout/stderr，可通过 `docker logs` 或二进制运行控制台查看。默认日志级别为 `info`，可使用 `PROXYGATEWAY_LOG_LEVEL` 调整为 `debug`、`info`、`warn` 或 `error`：

```bash
docker run -d \
  --name proxy-gateway \
  -p 8080:8080 \
  -v proxy-gateway-data:/data \
  -e PROXYGATEWAY_LOG_LEVEL=debug \
  ghcr.io/redshamea/proxy-gateway:latest
```

本地二进制运行示例：

```bash
PROXYGATEWAY_LOG_LEVEL=warn ./proxygateway
```

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
