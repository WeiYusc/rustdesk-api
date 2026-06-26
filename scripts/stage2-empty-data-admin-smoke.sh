#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
REPO=${REPO:-$(cd "$SCRIPT_DIR/.." && pwd)}
RUN_ROOT=${RUN_ROOT:-$(mktemp -d "${TMPDIR:-/tmp}/rustdesk-api-empty-data-admin.XXXXXX")}
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
  web-client: 0
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
  id-server: "127.0.0.1"
  relay-server: "127.0.0.1"
  api-server: "http://127.0.0.1:${PORT}"
  key: "test-public-key"
  key-file: "./data/id_ed25519.pub"
  personal: 1
  webclient-magic-queryonline: 0
  ws-host: ""
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

GOFLAGS=-mod=mod go build -o "$RUN_ROOT/apimain" ./cmd/apimain.go
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
password=$(grep -o 'Admin Password Is: [^ ]*' "$LOG" | tail -1 | awk '{print $4}')
if [[ -z "$password" ]]; then
  echo "failed to extract admin password" >&2
  echo "--- log ---" >&2
  cat "$LOG" >&2
  exit 1
fi
login=$(curl -fsS -X POST "http://127.0.0.1:${PORT}/api/admin/login" \
  -H 'Content-Type: application/json' \
  --data "{\"username\":\"admin\",\"password\":\"${password}\",\"platform\":\"stage2-smoke\"}")
python3 - <<PY
import json, sqlite3, pathlib
version=json.loads('''$version''')
login=json.loads('''$login''')
assert version['code'] == 0, version
assert login['code'] == 0, login
assert login['data']['token'], login
conn=sqlite3.connect('$RUN_ROOT/data/rustdeskapi.db')
users=conn.execute('select username, status, group_id from users order by id').fetchall()
versions=conn.execute('select version from versions').fetchall()
assert users and users[0][0] == 'admin', users
assert versions, versions
print('RUN_ROOT=$RUN_ROOT')
print('PORT=$PORT')
print('VERSION_RESPONSE=' + json.dumps(version, ensure_ascii=False))
print('LOGIN_CODE=' + str(login['code']))
print('LOGIN_AUTH_PRESENT=' + str(bool(login['data']['token'])))
print('DB_USERS=' + repr(users))
print('DB_VERSIONS=' + repr(versions))
PY
