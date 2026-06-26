# Compatibility Matrix

This document is the planning baseline for this fork. It records what is already
implemented, what appears partially covered, and what still needs source-level or
runtime verification before this project claims compatibility with newer RustDesk
clients and self-hosted deployments.

## Evidence Policy

Compatibility entries use only these evidence levels:

- **Verified**: covered by current source plus a local test, smoke run, or
  reproducible runtime check in this fork.
- **Source-present**: implementation exists in the current source tree, but this
  stage has not yet exercised it end to end.
- **Reference-only**: observed in another project or upstream discussion and must
  be reimplemented or validated independently before adoption.
- **Gap**: known missing, broken, stale, or intentionally deferred behavior.
- **Deferred**: intentionally outside the current API-server phase.

Do not upgrade an entry to **Verified** without recording the command, fixture, or
manual runtime path that proved it.

## Repository and License Boundary

| Source | Role in this fork | License boundary |
| --- | --- | --- |
| `alonginwind/rustdesk-api` `main` | Code base for this fork | Current implementation source |
| `lejianwen/rustdesk-api` | Upstream lineage, issue and PR source | MIT upstream lineage |
| `lantongxue/rustdesk-api-server-pro` | Behavior and deployment reference only | AGPL-3.0; do not copy code |
| `liyan-lucky/rustdesk-api-server-pro` | Behavior, compatibility, and documentation reference only | AGPL-3.0; do not copy code |
| `lejianwen/rustdesk-server` | Future server-integration reference | Deferred; do not merge in this phase |
| `rustdesk/rustdesk-server` | Future server base candidate | Deferred; expected base for later server work |

AGPL projects may inform requirements, endpoint behavior, compatibility gaps, and
operator experience. Their source code, UI implementation, controller logic,
assets, and text should not be copied into this fork unless the project license
strategy is deliberately changed.

## Current Baseline

- Base branch: `alonginwind/rustdesk-api` `main`.
- Baseline commit at stage start: `46e1e869fe04d1fd8a312f4bb207be9423549acb`.
- Project type: Go API server with Gin, GORM, Web Admin, and optional Web Client
  static assets.
- Current focus: RustDesk API compatibility and operational usability.
- Explicit non-goal for this stage: rebasing or merging `rustdesk-server`.

## Status Legend

| Status | Meaning | Required next action |
| --- | --- | --- |
| Verified | Source and behavior were exercised in this fork | Keep regression coverage current |
| Source-present | Source exists but behavior needs a focused test or smoke run | Add test/smoke evidence |
| Partial | Some behavior exists, but known versions or edge cases are missing | Define compatibility test and patch plan |
| Reference-only | Seen in reference projects or issues, not adopted yet | Re-spec and clean-room implement if needed |
| Gap | Missing or reported broken | Reproduce before fixing |
| Deferred | Out of current phase | Keep boundary documented |

## Client/API Compatibility Matrix

| Area | Endpoint or behavior | Current status | Evidence / notes | Next action |
| --- | --- | --- | --- | --- |
| Login | Account login for RustDesk client | Verified | HTTP smoke covers `/api/login` JSON flow, password validation, token/log creation, and existing peer binding; verified by `GOFLAGS=-mod=mod go test ./http/controller/api -run 'Test(Login|SysInfo|Heartbeat)' -count=1 -v` | Local full-process HTTP smoke passed; real RustDesk GUI/client-server connection remains skipped by user instruction |
| Login options | Client login option discovery | Source-present | API route set includes login options behavior inherited from upstream lineage | Verify response shape against current RustDesk client expectations |
| OAuth/OIDC | GitHub, Google, generic OIDC login | Verified | OIDC `email_verified` now accepts JSON bool, string, case/space variants, null, or missing values; verified by `GOFLAGS=-mod=mod go test ./model -run 'TestOidcUserEmailVerified' -count=1 -v` | Continue broader provider callback smoke tests before changing OAuth flow |
| LDAP | LDAP/AD login | Partial | Local helper tests verified configured/default user attributes, simple user search request construction, `ldap.Entry` mapping, `userAccountControl` disabled-bit handling, and direct `memberOf` admin/allow-group checks. External temporary fake-LDAP protocol smoke exercised admin bind, user search, user bind, `Authenticate`, and local user creation/mapping; observed user `alice`, `alice@example.com`, admin=true, status=enabled. Verified by `GOFLAGS=-mod=mod go test ./service -run 'TestLdap' -count=1 -v` plus an external temporary fake-LDAP protocol smoke archived outside this repository. Not verified: real AD/OpenLDAP, TLS/StartTLS/LDAPS, reverse group lookup, result cardinality edge cases, failure diagnostics, or end-to-end `/api/login` LDAP authentication | Keep #508/#509 open until live LDAP or committed fake-LDAP integration fixture covers bind/search failures and diagnostics |
| Heartbeat | Client heartbeat / online state | Verified | Controller tests cover deletion of logged-in unbound peers and preservation of logged-in peers with alias; verified by `GOFLAGS=-mod=mod go test ./http/controller/api -run 'Test(SysInfo|Heartbeat)' -count=1 -v` | Local full-process HTTP smoke passed; real RustDesk GUI/client-server connection remains skipped by user instruction |
| Sysinfo | Device information upload | Verified | Controller tests cover unattended peer creation, logged-in device `IGNORE`, and no overwrite of existing logged-in peer fields; verified by `GOFLAGS=-mod=mod go test ./http/controller/api -run 'Test(SysInfo|Heartbeat)' -count=1 -v` | Local full-process HTTP smoke passed; real RustDesk GUI/client-server connection remains skipped by user instruction |
| Peer list | Device list in admin/client contexts | Partial | `alonginwind/main` changes sorting, alias sync, and unattended-device filtering | Continue HTTP smoke for logged-in vs unattended sysinfo/heartbeat visibility |
| Peer login binding | Existing peer binding during account login | Verified | `PeerService.Update` now preserves explicit non-zero `user_id` during login binding while still allowing explicit clear-to-zero; verified by `GOFLAGS=-mod=mod go test ./service -run 'TestPeerService' -count=1 -v` | Use as evidence input for the broader connection-failure matrix |
| Address book | Address book CRUD and client sync | Source-present | Current fork has address book models, admin API, and client API | Add table-driven tests for owner, collection, alias, and duplicate cases |
| Address book alias | Alias synchronization into device list | Partial | `alonginwind/main` includes alias sync changes | Verify no private address-book alias leaks into global/admin views |
| Groups | Group management and client access | Source-present | Current code includes group models and routes inherited from upstream | Compare current client behavior and add compatibility notes |
| Device groups | RustDesk 1.3.8+ device group endpoints | Source-present | Upstream lineage contains device-group route support | Verify with request/response fixtures |
| Personal API | Personal-version API support | Partial | Personal address-book routes exist; `rustdesk.personal` gates specific personal address-book responses rather than the whole route group | Document supported mode and test disabled/enabled behavior |
| Audit logs | Connection and file-transfer audit endpoints | Source-present | Models and admin views exist for connection/file audit | Add API fixture tests before changing audit behavior |
| WebClient share | Temporary peer sharing for browser client | Verified | Empty or non-string `share_token` now returns controlled validation error instead of panic/500; verified by `GOFLAGS=-mod=mod go test ./http/controller/api -run 'TestWebClientSharedPeerRejectsInvalidShareToken' -count=1 -v` and local curl smoke | Continue security-reviewing valid share-token lifecycle and guest visibility |
| WebClient static route | Verified absent assets / route boundary | Full-process linux/amd64 smoke with current resources returned `/webclient/` 404 because `resources/web` is absent; this confirms no packaged WebClient app is present and no assets were restored | Keep as absent unless WebClient source/legal review explicitly approves assets |
| WebClient v2 | `/webclient2` / preview client | Gap | Removed from `alonginwind/main`; upstream had DMCA-related removal history | Do not restore without legal and source review |
| Token auth | API token / token expiry | Source-present | Upstream lineage added token expiry and token verify API | Add tests before extending token semantics |
| Server config | Partial | Auth-protected `/api/server-config*` behavior was checked earlier; full-process static smoke verified public `/webclient-config/index.js` includes API/WS host settings but does not expose configured `id_server` or `key` | Continue authenticated server-config fixtures before changing server discovery semantics |
| MUST_LOGIN | Server-side forced-login integration | Partial | API side exposes config expectations and `docs/current-fork-operations.md` documents the API/server boundary; full behavior depends on server fork | Defer implementation until the future rustdesk-server phase |
| Online notifications | Device online/offline notification | Reference-only | Requested in upstream issues; not established in current fork | Treat as future feature after event model is specified |
| Custom client ID | User-defined RustDesk ID | Reference-only | Requested upstream; not confirmed in current API source | Defer until official client/server constraints are understood |

## Admin and Operations Compatibility Matrix

| Area | Current status | Evidence / notes | Next action |
| --- | --- | --- | --- |
| Admin Web UI | Partial | Admin routes/controllers and documentation exist, but full-process smoke returned `/_admin/` 404 with current resources, confirming built admin static assets are not present in this source snapshot | Verify admin build/source provenance before major UI edits; do not claim packaged UI yet |
| Mobile admin usability | Partial | `alonginwind/main` includes mobile display adjustments | Add browser/mobile smoke checklist before UI changes |
| Docker image | Verified for linux/amd64 runtime smoke | `Dockerfile` builds from prebuilt `amd64/release`. A glibc-dynamic amd64 binary built successfully but failed in Alpine with `exec ./apimain: no such file or directory`; rebuilding with `musl-gcc` static CGO produced a working Alpine container, and `/api/version` returned success | Keep release workflow on musl/static CGO for Alpine images; do not use host glibc dynamic binaries |
| S6/full image | Verified for linux/amd64 startup smoke | `Dockerfile_full_s6` built against `rustdesk/rustdesk-server-s6:latest`; container logs show s6 starting `hbbr`, `hbbs`, and `api`, and `/api/version` returned success | For this fork's current scope, linux/amd64 S6 startup is sufficient; real RustDesk client/server connectivity remains a separate runtime acceptance task |
| OpenWrt package | Out of scope | Upstream users request OpenWrt packaging; reference projects document one-container deployment, but this fork currently targets local linux/amd64 only | Do not plan native/OpenWrt packaging unless scope changes |
| ARM64 image | Out of scope | CI workflow has linux/arm64 entries, but current project decision is not to support or validate non-amd64 targets | Ignore unless future distribution scope changes |
| i386 image | Out of scope | Upstream PR #445 proposes i386 support. Local 386 CGO failed due missing 32-bit libc headers, but non-amd64 is not a target for this fork | Do not spend effort on 386 support unless future distribution scope changes |
| SQLite deployment | Verified for empty-data linux/amd64 smoke | Temporary full-process run built `apimain`, booted with an empty SQLite data directory, auto-migrated schema version 265, created default `admin`, and `/api/version` plus `/api/admin/login` succeeded | Keep `scripts/stage2-empty-data-admin-smoke.sh` as the local smoke; MySQL/PostgreSQL remain separate |
| MySQL/PostgreSQL | Source-present | Config and ORM adapters exist | Add connection-string and TLS compatibility tests later |
| Redis/cache | Source-present | Config and cache abstractions exist | Verify optional behavior remains disabled by default |
| Config by env | Source-present | Config supports environment variable overrides | Document canonical env names and add config-load tests |
| Initial admin password | Verified for empty-data SQLite/linux-amd64 smoke | First-run migration creates default `admin` and logs a generated random password; local smoke uses it without recording the secret to prove `/api/admin/login` succeeds | Document that operators must capture the first-run log or use `reset-admin-pwd` |
| Password reset | Verified for local SQLite/linux-amd64 smoke | `scripts/stage2-admin-password-reset-smoke.sh` stops the temporary API, runs `reset-admin-pwd <temporary-password>`, restarts, verifies new-password login, and verifies old-password rejection | Add concise operator docs before release packaging |
| Logs/diagnostics | Partial | Existing logging exists, but LDAP and connection issues need better diagnostics | Add redacted structured diagnostics as Stage 1 candidate |

## Upstream Issue and PR Triage

These items should drive early stages because they map to user-visible failures or
small high-confidence fixes.

Triage rule: before acting on a `lejianwen/rustdesk-api` issue or PR, first check
whether `alonginwind/rustdesk-api` `main` already fixed, removed, or intentionally
changed the relevant behavior. Treat `alonginwind/main` as the current base, not
as a blank copy of `lejianwen/master`.

WebClient rule: WebClient resources have a DMCA-related removal history and the
current `alonginwind/main` source snapshot does not include `resources/web`.
Do not restore, vendor, or regenerate WebClient assets unless their source and
license/legal status are explicitly reviewed and approved.

| Source | Topic | Priority | Current classification | Proposed stage |
| --- | --- | --- | --- | --- |
| `lejianwen/rustdesk-api` PR #502 | OIDC `email_verified` may be string or boolean | Done | Implemented in this fork with focused tests | Stage 1.1 complete |
| `lejianwen/rustdesk-api` issue #500 | WebClient may expose previous server/API/key info when unauthenticated | Current path not reproduced | `alonginwind/main` removed WebClient assets; `/api/server-config*` is auth-protected; adjacent `/api/shared-peer` panic fixed | Stage 1.2 complete; do not restore WebClient assets without legal/source review |
| `lejianwen/rustdesk-api` issues #496/#514/#519 | Logged-in clients cannot connect / waiting for image transfer | Needs re-triage against `alonginwind/main` | `alonginwind/main` has peer/share/address-book changes including `只同步未登录的被控端` and `修复强制登录时的Peer分享`; next evidence target is login/sysinfo/heartbeat/peer visibility matrix, not immediate patching | Stage 1.3 evidence matrix |
| `lejianwen/rustdesk-api` issues #508/#509 | LDAP lookup failure and insufficient logs | Needs re-triage against `alonginwind/main` | `alonginwind/main` includes LDAP TLS, allow-group, and OIDC+LDAP fixes; need fixture or live LDAP reproduction | Stage 1.4 or after connection triage |
| `lejianwen/rustdesk-api` issue #506/#495 | WebClient missing text / 404 | Likely superseded by WebClient removal | `resources/web` is absent in current source; `/webclient/` 404 in local smoke | Defer until WebClient legal/source policy is resolved |
| `lejianwen/rustdesk-api` issue #504 | MUST_LOGIN unclear on Windows/direct zip deployments | Documentation/server-dependent | `alonginwind/main` includes `修复强制登录时的Peer分享`; API docs can improve, server behavior deferred | Stage 2 docs after server boundary review |
| `lejianwen/rustdesk-api` PR #445 | i386 support and S6 image upload fix | Locally resolved for amd64 scope | linux/amd64 Docker runtime and full S6 images build and start with musl/static CGO binary; glibc dynamic binary fails in Alpine, so release artifacts must be musl/static for Alpine. i386/ARM64/GHCR manifest concerns are out of scope for this fork's current local-amd64 target | Stage 2 complete for current scope; no non-amd64 follow-up unless project scope changes |
| `lejianwen/rustdesk-api` issue #520 | OpenWrt package request | Packaging expansion | Reference-only for now; do not overpromise native package | Out of scope unless distribution targets expand beyond local amd64 |

## Reference-Only Feature Backlog from API Server Pro Projects

The following are useful product ideas, but must be specified and implemented in
this fork without copying AGPL code.

| Feature idea | Why it matters | Adoption rule |
| --- | --- | --- |
| Single-container deployment with admin UI and API | Simplifies self-hosting and OpenWrt-like deployments | Recreate deployment flow using this repo's build artifacts |
| Compatibility placeholder endpoints for newer clients | Avoids 404s when clients probe enterprise/pro endpoints | Define minimal safe response shapes and tests first |
| Device/user/group management refinements | Improves admin usability | Reuse only behavior requirements, not UI/controller code |
| OAuth provider configuration UX | Makes OIDC/GitHub/Google setup easier | Implement through this repo's existing OAuth model |
| Mail, verification code, and notification flows | Useful for account operations | Defer until auth/account model is audited |
| Version capability matrix | Prevents overclaiming client compatibility | Build into this document and tests first |
| OpenWrt/one-container docs | Important operator experience | Write original docs based on tested artifacts |

## Future `rustdesk-server` Integration Boundary

Server integration is intentionally deferred. Current API-server work should only
prepare clean boundaries:

- Keep ID server, relay server, API server, public key, and forced-login settings
  explicit in configuration and server-config responses.
- Do not merge `lejianwen/rustdesk-server` code into this repository during API
  compatibility stages.
- When server work starts, use `rustdesk/rustdesk-server` as the expected base
  and port only the necessary API/JWT/MUST_LOGIN/WebSocket behavior after a
  separate Rust-source audit.
- Maintain a separate compatibility matrix for server protocol behavior once that
  phase begins.

## Phase Roadmap

### Stage 0: Compatibility Baseline

- Create this document.
- Audit license/source boundaries.
- Establish evidence vocabulary and first triage list.

Exit gate:

- `compatibility.md` exists and is internally consistent.
- Controller audit passes.
- Independent reviewer audit passes or findings are resolved/documented.

### Stage 1: High-Value Compatibility Fixes

Candidate tasks:

1. OIDC `email_verified` string/bool compatibility with regression tests.
2. WebClient unauthenticated information-leak reproduction and fix if confirmed.
3. Logged-in-client connection failure investigation using local runtime evidence.
4. LDAP diagnostics and lookup behavior reproduction plan.

Exit gate:

- Each fix has focused tests or a reproducible smoke check.
- Controller review and independent subagent review both pass.

### Stage 2: Packaging and Operator Experience

Candidate tasks:

1. Docker/S6 linux/amd64 smoke review.
2. Empty-data-dir migration and initial admin credential smoke test.
3. Static resource and WebClient packaging smoke test, without restoring removed WebClient assets.
4. API-side `MUST_LOGIN` and current fork operations boundary documentation; Windows zip remains out of scope for the current linux/amd64-only phase.

Exit gate:

- Build or smoke commands produce real evidence.
- Documentation matches tested artifact behavior.

### Stage 3: Newer Client Compatibility and Clean-Room Enhancements

Candidate tasks:

1. Use `docs/endpoint-compatibility-audit.md` as the clean-room route/capability baseline before adding endpoints.
2. Add safe placeholder endpoints only where newer clients actually probe them and fail.
3. Expand additional compatibility fixtures for existing routes after the first fixture sets (`server-config*`, `sysinfo_ver`, `device-group/accessible`, `sysinfo→heartbeat`, heartbeat empty boundaries, current-user/user-info parity, admin current/user-list/user-token/group/user/peer/tag/device-group/login-log/share-record/address-book-collection/address-book-collection-rule auth and CRUD boundaries, users/peers group visibility, address-book personal/shared/delete flows, audit conn/file write/update/admin flows, logout token/peer cleanup and same-UUID boundary) pass.
4. Add version capability notes and tests.
5. Consider notification/mail/account flows after auth model review.

Exit gate:

- No AGPL code copied.
- Each endpoint has request/response fixtures and compatibility notes.

### Stage 4: Future Server Fusion Planning

Candidate tasks:

1. Audit official `rustdesk/rustdesk-server` current protocol behavior.
2. Compare `lejianwen/rustdesk-server` API/JWT/MUST_LOGIN changes.
3. Design a separate Rust patch plan based on official server source.

Exit gate:

- Separate server plan exists.
- API repo remains decoupled from server implementation changes.

## Audit Checklist for Each Stage

- [ ] Scope matches the stage plan.
- [ ] Changed files are intentional.
- [ ] No AGPL source/code/text copied from reference projects.
- [ ] Claims are labeled as Verified, Source-present, Partial, Reference-only,
      Gap, or Deferred.
- [ ] Tests or smoke checks are recorded for every Verified claim.
- [ ] `git diff --check` passes.
- [ ] Controller has read changed files directly.
- [ ] Independent subagent audit has reviewed the stage output.
