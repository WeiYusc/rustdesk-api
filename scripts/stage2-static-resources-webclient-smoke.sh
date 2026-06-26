#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
REPO=${REPO:-$(cd "$SCRIPT_DIR/.." && pwd)}
RUN_ROOT=${RUN_ROOT:-$(mktemp -d "${TMPDIR:-/tmp}/rustdesk-api-static-resources.XXXXXX")}
PORT=${PORT:-$((22000 + RANDOM % 20000))}
LOG="$RUN_ROOT/runtime/log.txt"
STDOUT_LOG="$RUN_ROOT/runtime/stdout.log"
PID_FILE="$RUN_ROOT/runtime/apimain.pid"

cleanup() {
  if [[ -f "$RUN_ROOT/go.mod.before" ]]; then
    cp "$RUN_ROOT/go.mod.before" "$REPO/go.mod"
  fi
  if [[ -f "$RUN_ROOT/go.sum.before" ]]; then
    cp "$RUN_ROOT/go.sum.before" "$REPO/go.sum"
  elif [[ -f "$REPO/go.sum" && ! -f "$RUN_ROOT/go.sum.existed" ]]; then
    rm -f "$REPO/go.sum"
  fi
  if [[ -f "$PID_FILE" ]]; then
    pid=$(cat "$PID_FILE" || true)
    if [[ -n "${pid:-}" ]] && kill -0 "$pid" 2>/dev/null; then
      kill "$pid" 2>/dev/null || true
      for _ in $(seq 1 20); do
        kill -0 "$pid" 2>/dev/null || break
        sleep 0.1
      done
      kill -9 "$pid" 2>/dev/null || true
    fi
  fi
}
trap cleanup EXIT

mkdir -p "$RUN_ROOT/conf" "$RUN_ROOT/data" "$RUN_ROOT/runtime" "$RUN_ROOT/resources"
cp -a "$REPO/resources/i18n" "$RUN_ROOT/resources/"
cp -a "$REPO/resources/templates" "$RUN_ROOT/resources/"
cp -a "$REPO/resources/version" "$RUN_ROOT/resources/"
cp "$REPO/go.mod" "$RUN_ROOT/go.mod.before"
if [[ -f "$REPO/go.sum" ]]; then
  touch "$RUN_ROOT/go.sum.existed"
  cp "$REPO/go.sum" "$RUN_ROOT/go.sum.before"
fi

cat > "$RUN_ROOT/conf/config.yaml" <<YAML
lang: "en"
app:
  web-client: 1
  register: false
  register-status: 1
  captcha-threshold: -1
  ban-threshold: 0
  show-swagger: 0
  token-expire: 168h
  web-sso: false
  disable-pwd-login: false
admin:
  title: "API"
  hello-file: ""
  hello: ""
  id-server-port: 21116
  relay-server-port: 21117
gin:
  api-addr: "127.0.0.1:${PORT}"
  mode: "test"
  resources-path: "resources"
  trust-proxy: ""
gorm:
  type: "sqlite"
  max-idle-conns: 10
  max-open-conns: 100
mysql:
  username: ""
  password: ""
  addr: ""
  dbname: ""
  tls: "false"
postgresql:
  host: "127.0.0.1"
  port: "5432"
  user: ""
  password: ""
  dbname: "postgres"
  sslmode: "disable"
  time-zone: "Asia/Shanghai"
rustdesk:
  id-server: "stage2-id-secret.example"
  relay-server: "127.0.0.1"
  api-server: "http://127.0.0.1:${PORT}"
  key: "stage2-sensitive-public-key"
  key-file: "./data/id_ed25519.pub"
  personal: 1
  webclient-magic-queryonline: 0
  ws-host: "wss://stage2-ws.example"
logger:
  path: "./runtime/log.txt"
  level: "info"
  report-caller: false
proxy:
  enable: false
  host: "http://127.0.0.1:1080"
jwt:
  key: "stage2-smoke-jwt-key"
  expire-duration: 168h
ldap:
  enable: false
  url: "ldap://ldap.example.com:389"
  tls-ca-file: ""
  tls-verify: false
  base-dn: "dc=example,dc=com"
  bind-dn: "cn=admin,dc=example,dc=com"
  bind-password: "password"
  user:
    base-dn: "ou=users,dc=example,dc=com"
    enable-attr: ""
    enable-attr-value: ""
    filter: "(cn=*)"
    username: "uid"
    email: "mail"
    first-name: "givenName"
    last-name: "sn"
    sync: false
    admin-group: "cn=admin,dc=example,dc=com"
    allow-group: "cn=users,dc=example,dc=com"
cache:
  type: "file"
  file-dir: "./runtime/cache"
redis:
  addr: "127.0.0.1:6379"
  password: ""
  db: 0
oss:
  access-key-id: ""
  access-key-secret: ""
  host: ""
  callback-url: ""
  expire-time: 3600
  max-byte: 104857600
YAML

(
  cd "$REPO"
  GOFLAGS=-mod=mod go build -o "$RUN_ROOT/apimain" ./cmd/apimain.go
)
(
  cd "$RUN_ROOT"
  ./apimain -c ./conf/config.yaml >"$STDOUT_LOG" 2>&1 &
  echo $! > "$PID_FILE"
)

for _ in $(seq 1 100); do
  if grep -q "API SERVER START" "$LOG" 2>/dev/null && curl -fsS "http://127.0.0.1:${PORT}/api/version" >/dev/null 2>&1; then
    break
  fi
  sleep 0.1
done

version=$(curl -fsS "http://127.0.0.1:${PORT}/api/version")
config_status=$(curl -sS -o "$RUN_ROOT/runtime/webclient-config.js" -w '%{http_code}' "http://127.0.0.1:${PORT}/webclient-config/index.js")
webclient_status=$(curl -sS -o "$RUN_ROOT/runtime/webclient.html" -w '%{http_code}' "http://127.0.0.1:${PORT}/webclient/")
admin_status=$(curl -sS -o "$RUN_ROOT/runtime/admin.html" -w '%{http_code}' "http://127.0.0.1:${PORT}/_admin/")
upload_status=$(curl -sS -o "$RUN_ROOT/runtime/upload.txt" -w '%{http_code}' "http://127.0.0.1:${PORT}/upload/")
python3 - <<PY
import json, pathlib
version=json.loads('''$version''')
config=pathlib.Path('$RUN_ROOT/runtime/webclient-config.js').read_text()
webclient_status=int('''$webclient_status''')
admin_status=int('''$admin_status''')
upload_status=int('''$upload_status''')
config_status=int('''$config_status''')
if version.get('code') != 0:
    raise AssertionError('version endpoint failed')
if config_status != 200:
    raise AssertionError(f'webclient config status={config_status}')
if 'api-server' not in config or "ws2_prefix+'api-server'" not in config:
    raise AssertionError('webclient config missing api-server keys')
if 'stage2-id-secret.example' in config or 'stage2-sensitive-public-key' in config:
    raise AssertionError('webclient config leaked id_server or key')
if 'wss://stage2-ws.example' not in config:
    raise AssertionError('webclient config did not include configured ws_host')
if webclient_status != 404:
    raise AssertionError(f'unexpected /webclient/ status={webclient_status}; expected 404 while resources/web is absent')
if admin_status != 404:
    raise AssertionError(f'unexpected /_admin/ status={admin_status}; expected 404 while resources/admin is absent')
if upload_status not in (200, 301, 302, 403, 404):
    raise AssertionError(f'unexpected /upload/ status={upload_status}')
print('RUN_ROOT=$RUN_ROOT')
print('PORT=$PORT')
print('VERSION_CODE=' + str(version['code']))
print('WEBCLIENT_CONFIG_STATUS=' + str(config_status))
print('WEBCLIENT_CONFIG_HAS_API_SERVER=' + str('api-server' in config))
print('WEBCLIENT_CONFIG_LEAKS_ID_OR_KEY=False')
print('WEBCLIENT_STATUS=' + str(webclient_status))
print('ADMIN_STATIC_STATUS=' + str(admin_status))
print('UPLOAD_STATUS=' + str(upload_status))
PY
cleanup
trap - EXIT
if ! diff -q "$RUN_ROOT/go.mod.before" "$REPO/go.mod" >/dev/null; then
  echo "go.mod changed after smoke" >&2
  exit 1
fi
if [[ -f "$RUN_ROOT/go.sum.before" ]]; then
  if ! diff -q "$RUN_ROOT/go.sum.before" "$REPO/go.sum" >/dev/null; then
    echo "go.sum changed after smoke" >&2
    exit 1
  fi
elif [[ -f "$REPO/go.sum" ]]; then
  echo "go.sum was created after smoke" >&2
  exit 1
fi
