#!/usr/bin/env bash
# 修复验证：docs/nginx.conf 改完之后，POST /api/auth 仍吃 5r/m，GET /api/auth（会话校验）不受限。
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
work="$(mktemp -d)"
app_port=18556
ngx_port=18082

cleanup() {
  [[ -n "${ngx_pid:-}" ]] && kill "$ngx_pid" 2>/dev/null || true
  [[ -n "${app_pid:-}" ]] && kill "$app_pid" 2>/dev/null || true
  wait 2>/dev/null || true
  rm -rf "$work"
}
trap cleanup EXIT

cd "$repo"
go build -o "$work/app" .

DATA_DIR="$work/data" PORT="$app_port" HOST=127.0.0.1 TZ=Asia/Shanghai \
  ADMIN_PASSWORD=probe_pass_123 SESSION_SIGNING_KEY=probe_session_signing_key_123456 \
  "$work/app" >"$work/app.log" 2>&1 &
app_pid=$!
for _ in $(seq 100); do
  curl -fsS -o /dev/null "http://127.0.0.1:$app_port/livez" && break
  sleep 0.1
done

# 直连应用换 token（不过 nginx，不占限流额度）
token="$(curl -fsS -X POST -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"probe_pass_123"}' \
  "http://127.0.0.1:$app_port/api/auth" | python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])')"

# 把 docs/nginx.conf 转成能本机跑的形式：只动 TLS / 端口 / 上游，限流与 location 原样保留
python3 - "$repo/docs/nginx.conf" "$work/site.conf" "$ngx_port" "$app_port" <<'PY'
import sys
src, dst, ngx_port, app_port = sys.argv[1:5]
text = open(src).read().split("\nserver {\n    listen 80;")[0]
text = text.replace("listen 443 ssl;", f"listen {ngx_port};")
text = text.replace("server_name skin.example.com;", "server_name _;")
text = "\n".join(line for line in text.splitlines() if "ssl_certificate" not in line)
text = text.replace("http://127.0.0.1:8080", f"http://127.0.0.1:{app_port}")
open(dst, "w").write(text + "\n")
PY

mkdir -p "$work/tmp"
cat >"$work/main.conf" <<EOF
worker_processes 1;
error_log $work/nginx-error.log warn;
pid $work/nginx.pid;
events { worker_connections 64; }
http {
    access_log off;
    client_body_temp_path $work/tmp/client_body;
    proxy_temp_path $work/tmp/proxy;
    fastcgi_temp_path $work/tmp/fastcgi;
    uwsgi_temp_path $work/tmp/uwsgi;
    scgi_temp_path $work/tmp/scgi;
    include $work/site.conf;
}
EOF

nginx -t -c "$work/main.conf" -p "$work" >/dev/null
nginx -c "$work/main.conf" -p "$work" -g "daemon off;" >"$work/nginx.log" 2>&1 &
ngx_pid=$!
for _ in $(seq 100); do
  curl -fsS -o /dev/null "http://127.0.0.1:$ngx_port/livez" && break
  sleep 0.1
done

echo "POST /api/auth（错误密码，串行 6 次）："
post_codes=""
for _ in $(seq 6); do
  code="$(curl -sS -o /dev/null -w '%{http_code}' -X POST -H 'Content-Type: application/json' \
    -d '{"username":"admin","password":"wrong_password_123"}' \
    "http://127.0.0.1:$ngx_port/api/auth")"
  post_codes+="$code "
done
echo "  $post_codes"

echo "GET /api/auth（带 token，串行 8 次）："
get_codes=""
for _ in $(seq 8); do
  code="$(curl -sS -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $token" \
    "http://127.0.0.1:$ngx_port/api/auth")"
  get_codes+="$code "
done
echo "  $get_codes"

echo "GET /api/library（带 token，串行 3 次）："
for _ in $(seq 3); do
  code="$(curl -sS -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $token" \
    "http://127.0.0.1:$ngx_port/api/library")"
  printf '  %s' "$code"
done
echo
