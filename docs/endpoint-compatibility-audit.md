# Endpoint Compatibility Audit

本文件是 Stage 3 的入口审计，不实现新接口。目标是在新增任何占位 endpoint 前，先把当前 fork 已有路由、参考项目暴露的能力名称、以及需要真实客户端/协议证据的候选项分清楚。

## 审计边界

- 当前基底：`alonginwind/rustdesk-api` `main`，本地 HEAD `46e1e86` / tag `v2.9.3`。
- 参考项目：`lantongxue/rustdesk-api-server-pro`、`liyan-lucky/rustdesk-api-server-pro` 仅用于 endpoint/能力名称对比；不复制 AGPL 代码、响应体、UI 文案或实现逻辑。
- 当前项目目标仍为本机 `linux/amd64`；本文件不扩大架构范围。
- WebClient 资产和 server 侧 `MUST_LOGIN` 仍不在本阶段实现。

## 当前 API 路由概览

生成来源：`docs/api/api_swagger.json` 和 `http/router/api.go`。以源码路由为准；当前 Swagger 存在少量滞后/表达差异，例如 `currentUser`、`server-config`、`server-config-v2` 在源码中是 `POST`，而 Swagger 可能显示为 `GET` 或受 `basePath` 影响。

### 公共 / 半公共 API

| 方法 | 路径 | 备注 |
| --- | --- | --- |
| `GET` | `/api/` | 基础入口 |
| `GET` | `/api/version` | 版本信息，已在多次 smoke 中验证 |
| `POST` | `/api/heartbeat` | 客户端心跳 |
| `GET` | `/api/login-options` | 登录选项 |
| `POST` | `/api/login` | RustDesk 客户端登录 |
| `POST` | `/api/oidc/auth` | OIDC 登录发起 |
| `GET` | `/api/oidc/auth-query` | OIDC 查询 |
| `GET` | `/api/oauth/callback` / `/api/oauth/login` / `/api/oauth/msg` | OAuth 回调/消息 |
| `GET` | `/api/oidc/callback` / `/api/oidc/login` / `/api/oidc/msg` | OIDC 回调/消息 |
| `POST` | `/api/sysinfo` | 设备系统信息 |
| `POST` | `/api/sysinfo_ver` | 带版本系统信息 |
| `POST` | `/api/audit/conn` | 连接审计 |
| `POST` | `/api/audit/file` | 文件审计 |
| `POST` | `/api/shared-peer` | WebClient 分享访问；仅在 `app.web-client=1` 时注册；无效 `share_token` 已有回归测试 |
| `GET` | `/api/query-share-peer` | 查询分享 peer；仅在 `app.web-client=1` 时注册 |

### RustDesk 认证 API

这些路由位于 `frg.Use(middleware.RustAuth())` 之后，或单独使用 `middleware.RustAuth()`。

| 方法 | 路径 | 备注 |
| --- | --- | --- |
| `GET` | `/api/user/info` | 当前用户 |
| `POST` | `/api/currentUser` | 当前用户兼容路径 |
| `POST` | `/api/logout` | 登出 |
| `GET` | `/api/users` | 用户列表 |
| `GET` | `/api/peers` | peer 列表 |
| `GET` | `/api/device-group/accessible` | 可访问设备组 |
| `GET` | `/api/ab` | 地址簿 |
| `POST` | `/api/ab` | 更新地址簿 |
| `POST` | `/api/ab/personal` | 个人地址簿 |
| `POST` | `/api/ab/settings` | 地址簿设置 |
| `POST` | `/api/ab/shared/profiles` | 共享地址簿 profile |
| `POST` | `/api/ab/peers` | 地址簿 peer 列表 |
| `POST` | `/api/ab/tags/{guid}` | 标签列表 |
| `POST` | `/api/ab/peer/add/{guid}` | 添加 peer |
| `DELETE` | `/api/ab/peer/{guid}` | 删除 peer |
| `PUT` | `/api/ab/peer/update/{guid}` | 更新 peer |
| `POST` | `/api/ab/tag/add/{guid}` | 添加标签 |
| `PUT` | `/api/ab/tag/rename/{guid}` | 重命名标签 |
| `PUT` | `/api/ab/tag/update/{guid}` | 更新标签 |
| `DELETE` | `/api/ab/tag/{guid}` | 删除标签 |
| `POST` | `/api/server-config` | 仅在 `app.web-client=1` 时注册；路由级 `RustAuth`；返回 WebClient 所需 `id_server` / `key` / peers |
| `POST` | `/api/server-config-v2` | 仅在 `app.web-client=1` 时注册；路由级 `RustAuth`；返回 `id_server` / `key` |

## 当前 Admin 路由概览

生成来源：`docs/admin/admin_swagger.json` 和 `http/router/admin.go`。Swagger 当前列出 `101` 个 admin path，主要覆盖：

- 登录、登出、验证码、OIDC 登录；
- 用户、组、设备组、peer、标签；
- 地址簿、地址簿集合、集合规则；
- OAuth、登录日志、连接审计、文件审计；
- 用户 token、个人视图、文件上传；
- `/admin/config/admin`、`/admin/config/server`、`/admin/config/app`。

注意：当前源码快照没有 `resources/admin` 构建产物，`/_admin/` 静态入口在本地 smoke 中返回 `404`。这不代表 admin API 不存在，只代表内置 Web Admin 静态应用未随当前源码包提供。

## 参考项目能力名称（clean-room）

### `liyan-lucky/rustdesk-api-server-pro`

只读证据：`backend/app/controller/api/enterprise_compat.go` 和 `docs/PROJECT_DESCRIPTION.md`。可见的高层兼容 API 名称包括：

- 设备分组：`/api/device-groups`、`/api/device-groups/{guid}`、`/api/device-groups/{guid}/devices`。
- 用户分组：`/api/user-groups`、`/api/user-groups/{guid}`。
- 策略：`/api/strategies`、`/api/strategies/{guid}`、`/api/strategies/{guid}/status`、`/api/strategies/assign`。
- 设备管理兼容：`/api/devices`、`/api/devices/{guid}`、`/api/devices/{guid}/enable`、`/api/devices/{guid}/disable`、`/api/devices/{guid}/assign`。
- 用户管理兼容：`/api/users/{guid}`、`/api/users/{guid}/enable`、`/api/users/{guid}/disable`、`/api/users/disable_login_verification`、`/api/users/force-logout`、`/api/users/invite`、`/api/users/tfa/totp/enforce`。
- 其他兼容项：`/api/audit/alarm`、`/api/devices/cli`、`/lic/web/api/plugin-sign`。

这些名称说明参考项目尝试覆盖 RustDesk Pro / enterprise 风格探测面，但当前 fork 不应直接照搬实现。

### `lantongxue/rustdesk-api-server-pro`

只读扫描显示它同样是独立 AGPL 项目，包含 API server、用户 CLI、sync/start、版本 capability 等能力。当前扫描未从它提取到比上面列表更明确的 enterprise endpoint 候选；后续若要深入，应继续只读抽取路由名称，不复制实现。

## 初步差距分类

| 候选能力 | 当前 fork 状态 | 风险 / 约束 | 下一步门槛 |
| --- | --- | --- | --- |
| `/api/device-group/accessible` | 已存在 | 已有路径，不等同 enterprise CRUD | 补 request/response fixture |
| `/api/device-groups*` | 不存在 | 可能是新版/Pro 客户端探测；不能照搬 AGPL 实现 | 需要真实客户端 probe、日志或公开协议证据 |
| `/api/user-groups*` | 不存在 | 需要权限模型和用户组语义 | 先设计响应契约，不直接实现 |
| `/api/strategies*` | 不存在 | 策略模型复杂，误返回成功可能误导客户端 | 需要官方/客户端行为证据 |
| `/api/devices*` enterprise 管理路径 | 不存在；当前已有 `/api/peers` 和 admin peer API | 可能与现有 peer/device 模型重叠 | 先梳理当前 peer/device 字段映射 |
| `/api/audit/alarm` | 不存在 | 可能适合作为安全 no-op 接收端，但需要请求样例 | 需要 fixture 或客户端 probe |
| `/api/devices/cli` | 不存在 | 可能影响新版客户端 CLI 同步 | 需要请求样例和字段边界 |
| `/lic/web/api/plugin-sign` | 不存在 | 涉及 license/plugin 语义，不能伪装成真实签名服务 | 除非证明客户端因 404 中断，否则暂缓 |

## Stage 3 决策门槛

新增任何占位 endpoint 前必须满足：

1. 有真实 RustDesk 客户端日志、HTTP trace、公开协议文档或测试 fixture 证明客户端会探测该路径。
2. 明确该路径缺失导致用户可见失败，而不是可忽略的 404。
3. 定义 fail-safe 响应：不能返回会让客户端误以为高权限功能真实可用的成功状态。
4. 写入 request/response fixture 和自动化测试。
5. 记录该 endpoint 是 compatibility placeholder、partial implementation，还是完整功能。
6. 继续遵守 clean-room：AGPL 参考项目只提供需求名称，不提供实现来源。

## Fixture Evidence Added

Stage 3 已先覆盖当前存在的兼容路由，而不是新增 enterprise/pro 占位接口：

- `TestServerConfigRoutesRequireRustAuthAndExposeConfiguredServerValues`：在手工注册 `app.web-client=1` 分支下的控制器路由后，验证 `/api/server-config` 和 `/api/server-config-v2` 需要 `RustAuth`，并且认证后返回配置中的 `id_server` / `key`；同时确认 v1 包含 `peers` map，v2 不返回 `peers`。
- `TestSysInfoVerReturnsVersionAndStartTimeLines`：验证 `/api/sysinfo_ver` 返回 `200`，响应最后一行为启动时间格式。
- `TestDeviceGroupAccessibleRequiresAdminAndReturnsGroups`：验证 `/api/device-group/accessible` 需要管理员身份，非管理员被拒绝，管理员可以收到设备组列表。
- `TestSysInfoCreatedPeerHeartbeatUpdatesOnlineState`：验证 `/api/sysinfo` 创建未登录被控端后，`/api/heartbeat` 可识别该 peer 并更新在线时间/IP，响应保持空 JSON 对象。
- `TestHeartbeatReturnsEmptyObjectForInvalidPayloads`：验证 malformed JSON 和缺少 `uuid` 的 heartbeat 请求返回 `200 {}`。
- `TestHeartbeatReturnsEmptyObjectForUnknownPeerWithoutCreatingPeer`：验证未知 peer heartbeat 返回 `200 {}`，且不会创建 peer 记录。
- `TestUsersReturnsOnlySelfForDefaultGroupNonAdmin` / `TestPeersReturnsOnlyOwnedPeersForDefaultGroupNonAdmin`：验证 default group 非管理员只能看到自己和自己拥有的 peer。
- `TestUsersReturnsGroupMembersForShareGroupNonAdmin` / `TestPeersReturnsGroupPeersForShareGroupNonAdmin`：验证 share group 非管理员可以看到同组用户和同组 peer。
- `TestAddressBookPersonalAndSettingsExposePersonalGuid`：验证 `/api/ab/personal` 在 personal API 启用时返回当前用户 personal guid、用户名和 full-control rule，并验证 `/api/ab/settings` 当前返回 `max_peer_one_ab: 0`。
- `TestAddressBookTagAndPeerPersonalFlow`：验证 personal guid 下 tag add/list、peer add/update/list 的最小链路；`PeerUpdate` 只更新允许字段，未允许的 `hostname` 不被覆盖。
- `TestAuditConnCreatesUpdatesAndClosesExistingConnection`：验证 `/api/audit/conn` `action=new` 创建连接审计，空 action 只更新当前允许字段，`action=close` 设置 `close_time`。
- `TestAuditFileCreatesFileAuditWithInfoMetadata`：验证 `/api/audit/file` 创建文件审计，并从 JSON `info` 中派生 `from_name`、`ip` 和 `num` 字段。
- `TestLogoutRequiresAuthDeletesTokenAndUnbindsMatchingPeer`：验证 `/api/logout` 需要 RustAuth；认证登出返回当前 `200 null`，删除当前用户匹配 token，并仅解绑 token 记录中 `device_uuid` 对应且属于当前用户的 peer。
- `TestLogoutSameUuidDeletesOnlyCurrentTokenAndUnbindsFirstMatchingPeer`：验证同一用户存在同 UUID 多 token/多 peer 时，当前登出只删除当前 token，保留同 UUID 其他 token；peer 解绑遵循当前 `FindByUserIdAndUuid(...).First(...)` 单行行为，而不是按 UUID 批量解绑。
- `TestAddressBookSharedProfilesAndRulePrivileges`：验证 `/api/ab/shared/profiles` 返回自有 collection 的 full-control rule、个人共享 rule、群组 rule 覆盖较弱个人 rule 的最大权限；验证 read-only rule 可读不可写，read-write rule 可向共享 collection 添加 peer。
- `TestAdminAuditRoutesRequireAdminAndListWithFilters`：验证 admin audit list 路由需要 `api-token` + admin privilege，非管理员被拒绝；管理员可按 `peer_id` / `from_peer` 过滤并获得当前分页/list 响应形状。
- `TestAdminAuditDeleteAndBatchDeleteRemoveSelectedRows`：验证 admin audit conn/file 单条删除和批量删除只移除选中的审计行。
- `TestCurrentUserAndUserInfoRoutesReturnSameAuthenticatedPayload`：验证 `/api/user/info` 与 `/api/currentUser` 都需要 RustAuth，分别使用当前 GET/POST 方法，并返回相同的 `UserPayload` identity/status/admin/info shape。
- `TestAddressBookPeerDeleteRequiresFullControlAndDeletesSelectedOnly`：验证 DELETE `/api/ab/peer/:guid` 当前需要 full-control rule；read-write rule 删除被拒且不变，full-control 只删除选中 peer 并保留同 collection 其他 peer。
- `TestAdminUserCurrentUsesBackendAuthAndReturnsRoleRoutes`：验证 `/api/admin/user/current` 只需要后台 `api-token` 认证；缺少 token 返回响应 code `403`，非管理员可获取自己的登录 payload 和普通 route names，管理员获得 `*` route wildcard。
- `TestAdminUserListRequiresAdminPrivilegeAndReturnsUsers`：验证 `/api/admin/user/list` 在后台认证后还需要 admin privilege；非管理员被拒绝，管理员可按用户名过滤获得当前分页/list 响应形状。
- `TestAdminUserTokenListRequiresAdminAndFiltersByUser`：验证 `/api/admin/user_token/list` 需要后台认证 + admin privilege；管理员可按 `user_id` 过滤，响应包含当前分页字段和 `id desc` token 顺序。
- `TestAdminUserTokenDeleteAndBatchDeleteRemoveSelectedRows`：验证 `/api/admin/user_token/delete` 和 `/api/admin/user_token/batchDelete` 删除选中 token，并保留未选中的其他 token。
- `TestAdminGroupRoutesRequireAdminAndListGroups`：验证 `/api/admin/group/list` 需要后台认证 + admin privilege；管理员可获得当前分页/list 响应形状和现有 insertion order。
- `TestAdminGroupCreateDetailUpdateAndDeleteSelectedOnly`：验证 `/api/admin/group/create`、`detail/:id`、`update`、`delete` 的当前 CRUD 行为；删除只移除选中 group 并保留其他 group。
- `TestAdminUserCRUDRoutesRequireAdminPrivilege`：验证 `/api/admin/user/create` 等 admin-only user CRUD 路由需要后台认证 + admin privilege。
- `TestAdminUserCreateDetailUpdateAndDeleteSelectedOnly`：验证 `/api/admin/user/create`、`detail/:id`、`update`、`delete` 的当前用户元数据 CRUD 行为；创建使用当前默认密码行为，删除只移除选中非管理员用户并保留其他用户。
- `TestAdminPeerRoutesRequireAdminAndListFiltersPeers`：验证 `/api/admin/peer/list` 需要后台认证 + admin privilege；管理员可按 hostname 过滤，并按当前默认 `alias ASC` 排序获得分页/list 响应形状。
- `TestAdminPeerCreateDetailUpdateDeleteAndBatchDeleteSelectedOnly`：验证 `/api/admin/peer/create`、`detail/:id`、`update`、`delete`、`batchDelete` 的当前 peer CRUD 行为；update 可通过当前 service 零值路径清空 `user_id`，单删/批量删除只移除选中 peer。
- `TestAdminAddressBookCollectionRoutesRequireAdminAndListByUser`：验证 `/api/admin/address_book_collection/list` 需要后台认证 + admin privilege；管理员可按 `user_id` 过滤并获得当前分页/list 响应形状。
- `TestAdminAddressBookCollectionCreateDetailUpdateAndDeleteCascadesSelectedOnly`：验证 `/api/admin/address_book_collection/create`、`detail/:id`、`update`、`delete` 的当前 collection CRUD 行为；删除会按当前 service 级联清理选中 collection 下的 rules/address-book rows，并保留其他 collection 及其关联行。
- `TestAdminAddressBookCollectionRuleRoutesRequireAdminAndListFiltersRules`：验证 `/api/admin/address_book_collection_rule/list` 需要后台认证 + admin privilege；管理员可按 `user_id` 和 `collection_id` 过滤并获得当前分页/list 响应形状。
- `TestAdminAddressBookCollectionRuleCreateValidatesOwnerTargetsAndDuplicates`：验证 `/api/admin/address_book_collection_rule/create` 的当前 owner 校验、个人/群组 share-target 存在性校验、自分享拒绝、重复规则拒绝，以及有效群组分享创建。
- `TestAdminAddressBookCollectionRuleDetailUpdateAndDeleteSelectedOnly`：验证 `/api/admin/address_book_collection_rule/detail/:id`、`update`、`delete` 的当前 rule CRUD 行为；update 会拒绝与其他规则冲突，delete 只移除选中 rule。
- `TestAdminTagRoutesRequireAdminAndListFiltersTags`：验证 `/api/admin/tag/list` 需要后台认证 + admin privilege；管理员可按 `user_id` 和 `collection_id` 过滤，响应包含当前分页/list shape 和 collection preload 字段。
- `TestAdminTagCreateDetailUpdateAndDeleteSelectedOnly`：验证 `/api/admin/tag/create`、`detail/:id`、`update`、`delete` 的当前 tag CRUD 行为；create 缺少 `user_id` 被拒，delete 只移除选中 tag。
- `TestAdminDeviceGroupRoutesRequireAdminAndListGroups`：验证 `/api/admin/device_group/list` 需要后台认证 + admin privilege；管理员可获得当前分页/list 响应形状和现有 insertion order。
- `TestAdminDeviceGroupCreateDetailUpdateAndDeleteSelectedOnly`：验证 `/api/admin/device_group/create`、`detail/:id`、`update`、`delete` 的当前 device-group CRUD 行为；delete 只移除选中 device group。
- `TestAdminLoginLogRoutesRequireAdminAndListFiltersByUser`：验证 `/api/admin/login_log/list` 需要后台认证 + admin privilege；管理员可按 `user_id` 过滤，并按当前 `id desc` 排序获得分页/list 响应形状。
- `TestAdminLoginLogDeleteAndBatchDeleteRemoveSelectedRows`：验证 `/api/admin/login_log/delete` 和 `/api/admin/login_log/batchDelete` 删除选中 login log，并保留未选中 login log。
- `TestAdminShareRecordRoutesRequireAdminAndListFiltersByUser`：验证 `/api/admin/share_record/list` 需要后台认证 + admin privilege；管理员可按 `user_id` 过滤并获得当前分页/list 响应形状。
- `TestAdminShareRecordDeleteAndBatchDeleteRemoveSelectedRows`：验证 `/api/admin/share_record/delete` 和 `/api/admin/share_record/batchDelete` 删除选中 share record，并保留未选中 share record。

这些测试只验证当前 API 行为，不声明真实 RustDesk GUI 客户端端到端兼容。

## 推荐下一片段

在考虑 enterprise/pro placeholder 前，建议继续补当前已有兼容面的 request/response fixture：

1. 用真实 RustDesk 客户端或可复现 API probe 采集 enterprise/pro placeholder 404/行为证据；
2. 只有确认新版客户端实际探测并失败的 endpoint，才设计最小安全 placeholder。
