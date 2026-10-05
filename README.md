# RustDesk API / RustDesk API 服务

> 2026-10-05 发布前状态：维护源码已提交到 GitHub，但 GHCR `latest` 尚未包含本次修复。固定发布目标为 `maintenance-20261005-76ffa535`（尚未发布），通过构建/smoke及独立审查后按维护者授权更新 `latest`，不覆盖 `v0.1.0`。见 [维护发布说明](https://github.com/WeiYusc/rustdesk-server/blob/master/docs/full-s6/release-notes-maintenance-20261005.zh-CN.md)。

> Pre-publication status (2026-10-05): maintenance source is committed on GitHub, but GHCR `latest` does not yet contain these fixes. The intended pinned release is `maintenance-20261005-76ffa535` (not yet published); after build/smoke and independent review, update `latest` under maintainer authorization without replacing `v0.1.0`. See [maintenance release notes](https://github.com/WeiYusc/rustdesk-server/blob/master/docs/full-s6/release-notes-maintenance-20261005.en.md).


[中文](#中文) · [English](#english)

## 中文

`WeiYusc/rustdesk-api` 是 RustDesk 自托管管理栈的 API 服务，提供账号、地址簿、设备、审计、OAuth/LDAP、管理后台 API 和 RustDesk 客户端所需接口。

### 三仓库架构

| 仓库 | 作用 |
| --- | --- |
| `WeiYusc/rustdesk-api` | API 服务、账号体系、地址簿、审计和管理接口 |
| `WeiYusc/rustdesk-api-web` | Web Admin 前端，构建后放入 `resources/admin` |
| `WeiYusc/rustdesk-server` | `hbbs` / `hbbr` 服务端；full-s6 集成镜像构建入口 |

当前 API 源码包本身不跟踪构建后的 `resources/admin` 或 `resources/web` 静态资源。单独运行本仓库时，除非你手动构建并复制前端产物，否则 `/_admin/` 不会开箱即用。server 仓库的 full-s6 构建会自动构建并注入 Web Admin。

### 主要功能

- RustDesk 客户端 API：登录、设备信息、心跳、地址簿、分组等。
- Web Admin API：用户、设备、地址簿、标签、群组、OAuth、登录日志、连接/文件审计、服务端命令。
- 认证与账号：本地账号、OIDC/GitHub/Google OAuth、LDAP/AD。
- CLI：重置管理员密码。
- SQLite 默认运行；也支持 MySQL 等配置项。

> WebClient 说明：当前源码快照不包含 `resources/web`，不要把 `/webclient/` 或 `/webclient2` 作为已恢复/已发布功能来宣传。恢复 WebClient 前需要单独完成来源、许可证和合规审查。

### 配置

主要配置文件：[`conf/config.yaml`](conf/config.yaml)。环境变量前缀为 `RUSTDESK_API_`，例如：

| 变量 | 说明 | 示例 |
| --- | --- | --- |
| `RUSTDESK_API_LANG` | API/Web Admin 语言 | `zh-CN` / `en` |
| `RUSTDESK_API_GORM_TYPE` | 数据库类型 | `sqlite` |
| `RUSTDESK_API_RUSTDESK_ID_SERVER` | RustDesk ID server | `id.example.com:21116` |
| `RUSTDESK_API_RUSTDESK_RELAY_SERVER` | RustDesk relay server | `relay.example.com:21117` |
| `RUSTDESK_API_RUSTDESK_API_SERVER` | API 对外地址 | `https://api.example.com` |
| `RUSTDESK_API_RUSTDESK_KEY_FILE` | server 公钥文件 | `/data/id_ed25519.pub` |
| `RUSTDESK_API_JWT_KEY` | 与 server forced-login 共享的 JWT 密钥 | 生成的长随机字符串 |

### Docker 运行 API（单独 API 服务）

```bash
docker run -d \
  --name rustdesk-api \
  -p 21114:21114 \
  -v rustdesk-api-data:/app/data \
  -e TZ=Asia/Shanghai \
  -e RUSTDESK_API_LANG=zh-CN \
  -e RUSTDESK_API_RUSTDESK_ID_SERVER=id.example.com:21116 \
  -e RUSTDESK_API_RUSTDESK_RELAY_SERVER=relay.example.com:21117 \
  -e RUSTDESK_API_RUSTDESK_API_SERVER=https://api.example.com \
  -e RUSTDESK_API_RUSTDESK_KEY_FILE=/app/data/id_ed25519.pub \
  rustdesk-api:local
```

镜像名请替换为你实际构建或发布的 API 镜像。首次启动会创建 `admin` 用户并在日志中打印随机初始密码；请安全保存或使用 CLI 重置。

### 构建 Web Admin 并注入 API

```bash
cd /path/to/rustdesk-api-web
pnpm install --frozen-lockfile
pnpm build

mkdir -p /path/to/rustdesk-api/resources/admin
cp -a dist/. /path/to/rustdesk-api/resources/admin/
```

之后 API 会通过 `/_admin/` 提供 Web Admin。

### full-s6 集成部署（推荐完整栈）

完整单容器集成部署由 `WeiYusc/rustdesk-server` 仓库构建并发布。普通部署建议优先使用 server 仓库的 GHCR 镜像与 Compose 文档；本仓库仍保留单独 API 运行和本地构建方式。

```bash
RUSTDESK_API_SOURCE_DIR=/path/to/rustdesk-api \
RUSTDESK_API_WEB_SOURCE_DIR=/path/to/rustdesk-api-web \
RUSTDESK_FULL_S6_IMAGE=rustdesk-server-full-s6:local \
./scripts/build-full-s6-image.sh
```

当前状态：

- `linux/amd64` full-s6 稳定镜像已在 GHCR 发布：`ghcr.io/weiyusc/rustdesk-server-full-s6:v0.1.0`。
- `latest` 是稳定版浮动标签；`preview` 保留为预览通道，不自动等同稳定版。
- 本仓库源码包本身仍不包含构建后的 Web Admin；full-s6 镜像会从 `rustdesk-api-web` 构建并注入 Web Admin。
- 部署、Compose、升级和发布边界以 server 仓库 `docs/full-s6/` 为准。

### CLI

```bash
./apimain -h
./apimain reset-admin-pwd <new-password>
```

如果使用配置文件运行，请带上 `-c`：

```bash
./apimain -c ./conf/config.yaml reset-admin-pwd <new-password>
```

### 验证边界

已验证内容和未完成边界见：

- [docs/current-fork-operations.md](docs/current-fork-operations.md)
- [compatibility.md](compatibility.md)

## English

`WeiYusc/rustdesk-api` is the API service for a self-hosted RustDesk management stack. It provides accounts, address books, devices, audit logs, OAuth/LDAP, Web Admin APIs, and RustDesk client-facing endpoints.

### Three-repository architecture

| Repository | Role |
| --- | --- |
| `WeiYusc/rustdesk-api` | API service, accounts, address books, audit logs, admin endpoints |
| `WeiYusc/rustdesk-api-web` | Web Admin frontend copied to `resources/admin` after build |
| `WeiYusc/rustdesk-server` | `hbbs` / `hbbr` server and full-s6 image build entrypoint |

This API source checkout does not track built `resources/admin` or `resources/web` static assets. When running this repository alone, `/_admin/` is not available until you build and copy the frontend assets yourself. The server repository full-s6 build can build and inject Web Admin automatically.

### Main features

- RustDesk client API: login, sysinfo, heartbeat, address books, groups.
- Web Admin API: users, devices, address books, tags, groups, OAuth, login logs, connection/file audit logs, server commands.
- Authentication and accounts: local accounts, OIDC/GitHub/Google OAuth, LDAP/AD.
- CLI: reset admin password.
- SQLite by default; MySQL and other configuration options are available.

> WebClient note: the current source snapshot does not include `resources/web`. Do not advertise `/webclient/` or `/webclient2` as restored/published features until source, license, and compliance review is completed.

### Configuration

Main config file: [`conf/config.yaml`](conf/config.yaml). Environment variables use the `RUSTDESK_API_` prefix, for example:

| Variable | Description | Example |
| --- | --- | --- |
| `RUSTDESK_API_LANG` | API/Web Admin language | `zh-CN` / `en` |
| `RUSTDESK_API_GORM_TYPE` | Database type | `sqlite` |
| `RUSTDESK_API_RUSTDESK_ID_SERVER` | RustDesk ID server | `id.example.com:21116` |
| `RUSTDESK_API_RUSTDESK_RELAY_SERVER` | RustDesk relay server | `relay.example.com:21117` |
| `RUSTDESK_API_RUSTDESK_API_SERVER` | Public API URL | `https://api.example.com` |
| `RUSTDESK_API_RUSTDESK_KEY_FILE` | Server public key file | `/data/id_ed25519.pub` |
| `RUSTDESK_API_JWT_KEY` | JWT secret shared with forced-login server mode | long random string |

### Run API with Docker only

```bash
docker run -d \
  --name rustdesk-api \
  -p 21114:21114 \
  -v rustdesk-api-data:/app/data \
  -e TZ=Asia/Shanghai \
  -e RUSTDESK_API_LANG=en \
  -e RUSTDESK_API_RUSTDESK_ID_SERVER=id.example.com:21116 \
  -e RUSTDESK_API_RUSTDESK_RELAY_SERVER=relay.example.com:21117 \
  -e RUSTDESK_API_RUSTDESK_API_SERVER=https://api.example.com \
  -e RUSTDESK_API_RUSTDESK_KEY_FILE=/app/data/id_ed25519.pub \
  rustdesk-api:local
```

Replace the image name with the API image you actually built or published. First boot creates the `admin` user and prints a random initial password in logs. Capture it securely or reset it with the CLI.

### Build and inject Web Admin

```bash
cd /path/to/rustdesk-api-web
pnpm install --frozen-lockfile
pnpm build

mkdir -p /path/to/rustdesk-api/resources/admin
cp -a dist/. /path/to/rustdesk-api/resources/admin/
```

The API then serves Web Admin at `/_admin/`.

### full-s6 integrated deployment (recommended full stack)

The complete single-container stack is built and published from the `WeiYusc/rustdesk-server` repository. For normal deployments, prefer the server repository GHCR image and Compose documentation; this repository still documents API-only and local-build flows.

```bash
RUSTDESK_API_SOURCE_DIR=/path/to/rustdesk-api \
RUSTDESK_API_WEB_SOURCE_DIR=/path/to/rustdesk-api-web \
RUSTDESK_FULL_S6_IMAGE=rustdesk-server-full-s6:local \
./scripts/build-full-s6-image.sh
```

Current status:

- The `linux/amd64` full-s6 stable image is published on GHCR: `ghcr.io/weiyusc/rustdesk-server-full-s6:v0.1.0`.
- `latest` is the moving stable tag; `preview` remains a separate preview channel and is not automatically equivalent to stable.
- This API source checkout still does not include built Web Admin assets; the full-s6 image builds `rustdesk-api-web` and injects Web Admin.
- Deployment, Compose, upgrade, and release boundaries live in the server repository under `docs/full-s6/`.

### CLI

```bash
./apimain -h
./apimain reset-admin-pwd <new-password>
```

When using a config file:

```bash
./apimain -c ./conf/config.yaml reset-admin-pwd <new-password>
```

### Verification boundary

See:

- [docs/current-fork-operations.md](docs/current-fork-operations.md)
- [compatibility.md](compatibility.md)
