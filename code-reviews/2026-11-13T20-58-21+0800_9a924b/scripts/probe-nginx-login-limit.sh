#!/usr/bin/env bash
# 探针：按仓库里的 docs/nginx.conf 起一个真实反向代理，验证「/api/auth/login」这个
# location 在接口路径合并成 /api/auth 之后还剩不剩作用。
#
# 做法：把 docs/nginx.conf 的 TLS server 块改成本机高位端口（其余指令原样保留），
# 起 app + nginx，然后并发打 6 次 POST /api/auth（错误密码），看 nginx 有没有用
# limit_req zone=login 拦下（默认 503）。
# 对照组只改一行：location = /api/auth/login → location = /api/auth。
#
# 运行：bash code-reviews/2026-11-13T20-58-21+0800_9a924b/scripts/probe-nginx-login-limit.sh
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
work="$(mktemp -d)"
app_port="${APP_PORT:-18556}"
ngx_port="${NGX_PORT:-18081}"

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
  if curl -fsS -o /dev/null "http://127.0.0.1:$app_port/livez"; then break; fi
  sleep 0.1
done

# 把 docs/nginx.conf 转成能在本机跑的形式（只动 TLS/端口/上游，location 一律保留原样）
python3 - "$repo/docs/nginx.conf" "$work/site.conf" "$ngx_port" "$app_port" <<'PY'
import sys

src, dst, ngx_port, app_port = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4]
text = open(src).read()

# 去掉第二个 server 块（HTTP→HTTPS 跳转），它监听 80 且与本探针无关
text = text.split("\nserver {\n    listen 80;")[0]

text = text.replace("listen 443 ssl;", f"listen {ngx_port};")
text = text.replace("server_name skin.example.com;", "server_name _;")
text = "\n".join(line for line in text.splitlines() if "ssl_certificate" not in line)
text = text.replace("http://127.0.0.1:8080", f"http://127.0.0.1:{app_port}")
open(dst, "w").write(text + "\n")
PY

mkdir -p "$work/tmp"

write_main_conf() {
  local site="$1" main="$2"
  cat >"$main" <<EOF
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
    include $site;
}
EOF
}

run_nginx() {
  local main="$1"
  nginx -t -c "$main" -p "$work" >/dev/null 2>&1 || { echo "nginx 配置校验失败"; cat "$work/nginx-error.log" 2>/dev/null; return 1; }
  nginx -c "$main" -p "$work" -g "daemon off;" >"$work/nginx.log" 2>&1 &
  ngx_pid=$!
  for _ in $(seq 50); do
    if curl -sS -o /dev/null "http://127.0.0.1:$ngx_port/"; then return 0; fi
    sleep 0.1
  done
  return 1
}

hit() {
  # 6 次错误密码登录，打印每次的状态码（-m 5 防止 1s 恒定耗时叠加太久）
  for _ in $(seq 6); do
    curl -sS -m 5 -o /dev/null -w '%{http_code} ' -X POST \
      -H 'Content-Type: application/json' \
      -d '{"username":"probe","password":"wrong-password"}' \
      "http://127.0.0.1:$ngx_port/api/auth"
  done
  echo
}

echo "== 对照组 A：docs/nginx.conf 原样（location = /api/auth/login）"
cp "$work/site.conf" "$work/site-a.conf"
write_main_conf "$work/site-a.conf" "$work/nginx-a.conf"
if run_nginx "$work/nginx-a.conf"; then
  echo -n "   6 次 POST /api/auth -> "
  hit
  kill "$ngx_pid" 2>/dev/null || true; wait "$ngx_pid" 2>/dev/null || true; ngx_pid=""
else
  echo "   起 nginx 失败"
fi

echo "== 对照组 B：只把 location 改成 /api/auth"
sed 's|location = /api/auth/login|location = /api/auth|' "$work/site.conf" >"$work/site-b.conf"
grep -n 'location = /api/auth' "$work/site-b.conf" | head -3
write_main_conf "$work/site-b.conf" "$work/nginx-b.conf"
if run_nginx "$work/nginx-b.conf"; then
  echo -n "   6 次 POST /api/auth -> "
  hit
  kill "$ngx_pid" 2>/dev/null || true; wait "$ngx_pid" 2>/dev/null || true; ngx_pid=""
else
  echo "   起 nginx 失败"
fi
