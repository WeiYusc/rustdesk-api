# RustDesk API / RustDesk API 服务

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

### full-s6 集成部署（保留方案）

完整单容器集成部署由 `WeiYusc/rustdesk-server` 仓库构建：

```bash
RUSTDESK_API_SOURCE_DIR=/path/to/rustdesk-api \
RUSTDESK_API_WEB_SOURCE_DIR=/path/to/rustdesk-api-web \
RUSTDESK_FULL_S6_IMAGE=rustdesk-server-full-s6:local \
./scripts/build-full-s6-image.sh
```

当前状态：

- 本地 full-s6 构建和 smoke 已通过。
- 公共 Docker Hub/GHCR 镜像尚未发布。
- 公开镜像完成前，文档中的 full-s6 运行方式仅作为“保留方案/本地构建方案”。

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

### full-s6 integrated deployment (reserved option)

The complete single-container stack is built from the `WeiYusc/rustdesk-server` repository:

```bash
RUSTDESK_API_SOURCE_DIR=/path/to/rustdesk-api \
RUSTDESK_API_WEB_SOURCE_DIR=/path/to/rustdesk-api-web \
RUSTDESK_FULL_S6_IMAGE=rustdesk-server-full-s6:local \
./scripts/build-full-s6-image.sh
```

Current status:

- Local full-s6 build and smoke tests pass.
- No public Docker Hub/GHCR image has been published yet.
- Until the public image is ready, full-s6 runtime docs are a reserved/local-build option.

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
