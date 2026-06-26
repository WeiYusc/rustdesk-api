# Current Fork Operations Boundary

本文记录当前 fork 在本阶段已经验证的运行边界。它不是完整部署手册；未验证的目标不会在这里标为支持。

## 当前目标范围

- 当前本地验证目标仅为 `linux/amd64`。
- `i386`、`arm64`、OpenWrt、Windows zip、多架构 manifest/GHCR 推送均不在当前阶段范围内。
- 真实 RustDesk GUI 客户端连通性、图形化 Web Admin、WebClient 资产恢复、以及 `rustdesk-server` 侧协议融合，均属于后续阶段。

## 已验证的本机运行方式

### Alpine / Docker runtime

- `Dockerfile` 可以使用预构建的 `amd64/release` 目录构建运行镜像。
- Alpine 运行镜像不能使用宿主 glibc 动态链接的 `apimain`，否则容器内会出现 `exec ./apimain: no such file or directory`。
- 当前通过的路径是使用 `musl-gcc` 构建静态 CGO amd64 二进制，再放入 Alpine 镜像。
- `Dockerfile_full_s6` 的本机 amd64 启动 smoke 已通过：s6 启动 `hbbr`、`hbbs`、`api`，并且 `/api/version` 返回成功。

### SQLite 首次启动

本阶段已用临时目录验证：

- 空 `data/` 目录启动会执行迁移，当前数据库版本为 `265`。
- 首次迁移会创建默认管理员用户 `admin`。
- 初始密码是随机生成的，只写入启动日志；不要把该密码写入文档、issue 或公开日志。
- `/api/version` 和 `/api/admin/login` 已在本机临时运行目录中验证通过。

可复现 smoke：

```bash
bash scripts/stage2-empty-data-admin-smoke.sh
```

该脚本只使用临时运行目录，并在结束时停止临时进程。

### 管理员密码重置

本阶段已验证 `reset-admin-pwd` 对同一份 SQLite 配置/数据目录生效：

1. 首次启动创建默认 `admin`。
2. 用启动日志中的随机密码登录成功。
3. 停止临时 API 进程。
4. 在同一运行目录执行：

   ```bash
   ./apimain -c ./conf/config.yaml reset-admin-pwd <new-password>
   ```

5. 重启 API 后，新密码登录成功，旧随机密码被拒绝。

可复现 smoke：

```bash
bash scripts/stage2-admin-password-reset-smoke.sh
```

## 当前静态资源边界

当前源码快照包含运行 API 所需的：

- `resources/i18n`
- `resources/templates`
- `resources/version`

但本阶段 smoke 确认当前包内没有内置构建产物：

- `resources/admin` 不存在，因此 `/_admin/` 返回 `404`。
- `resources/web` 不存在，因此 `/webclient/` 返回 `404`。

这意味着当前 fork 不能声称“开箱即用内置 Web Admin UI / WebClient 静态应用”。如需启用 Web Admin，应按前端项目来源单独构建并放入 `resources/admin`；如需恢复 WebClient，必须先完成来源、许可证和 DMCA 风险审查。

可复现 smoke：

```bash
bash scripts/stage2-static-resources-webclient-smoke.sh
```

该脚本还验证公开的 `/webclient-config/index.js`：

- 返回 `200`。
- 包含 API server 与 WS host 设置。
- 不包含配置中的 `id_server` 或 `key`。

## Server config 与 `MUST_LOGIN` 边界

API 侧配置项包括：

- `rustdesk.id-server` / `RUSTDESK_API_RUSTDESK_ID_SERVER`
- `rustdesk.relay-server` / `RUSTDESK_API_RUSTDESK_RELAY_SERVER`
- `rustdesk.api-server` / `RUSTDESK_API_RUSTDESK_API_SERVER`
- `rustdesk.key` / `RUSTDESK_API_RUSTDESK_KEY`
- `jwt.key` / `RUSTDESK_API_JWT_KEY`

当前代码中：

- `/api/server-config` 和 `/api/server-config-v2` 通过路由级 `RustAuth` 中间件保护。
- 公开的 `/webclient-config/index.js` 只为 WebClient 写入 API/WS host 类设置，不公开 `id_server` 或 `key`。
- `MUST_LOGIN` 不是本 API 服务单独完成的功能；它依赖 server 侧支持。当前阶段只记录 API 配置和认证边界，不声称已经实现完整强制登录链路。

后续若进入 server 阶段，应以官方 `rustdesk/rustdesk-server` 为基底，单独审计并迁移必要的 API/JWT/`MUST_LOGIN`/WebSocket 行为。

## 验证脚本注意事项

本仓库当前存在一个 Go module 状态细节：`GOFLAGS=-mod=mod` 可能把 `go.mod` 中非版本化的 `master` / `main` 引用临时改写成版本号。已有 smoke 脚本会在临时构建后恢复 `go.mod` 和可选 `go.sum`，并检查它们没有保留漂移。

如果手工运行 Go 测试后看到 `go.mod` 被改写，先确认这是否只是该已知工具行为；不要把它作为本阶段功能改动提交。
