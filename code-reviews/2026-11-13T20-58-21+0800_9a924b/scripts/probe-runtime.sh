#!/usr/bin/env bash
# 运行时探针（评审用，不属于生产代码）：
#   1. 用当前源码编译出的二进制起一个独立实例；
#   2. 走 POST /api/auth 登录、GET /api/auth 取会话，确认路径合并后的行为；
#   3. 上传一张 64×64 PNG，观察 asset.upload 审计记录里的字段名（assetId vs asset_id）；
#   4. 用超长 username 打一次失败登录，观察审计记录的字段长度。
#
# 运行：bash code-reviews/2026-11-13T20-58-21+0800_9a924b/scripts/probe-runtime.sh
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
work="$(mktemp -d)"
log="$work/server.log"
port="${PROBE_PORT:-18555}"

cleanup() {
  if [[ -n "${pid:-}" ]]; then kill "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true; fi
  rm -rf "$work"
}
trap cleanup EXIT

cd "$repo"

# 1. 编译（复用 go build，不走前端）
go build -o "$work/app" .

# 2. 起实例：DATA_DIR 指向临时目录，凭据从环境变量给
DATA_DIR="$work/data" PORT="$port" HOST=127.0.0.1 TZ=Asia/Shanghai \
  ADMIN_USERNAME=probe ADMIN_PASSWORD=probe_pass_123 \
  SESSION_SIGNING_KEY=probe_session_signing_key_123456 \
  "$work/app" >"$log" 2>&1 &
pid=$!
for _ in $(seq 100); do
  if curl -fsS -o /dev/null "http://127.0.0.1:$port/livez"; then break; fi
  sleep 0.1
done

base="http://127.0.0.1:$port"

echo "== GET /api/auth（无 token）"
curl -sS -o /dev/null -w 'status=%{http_code} www-authenticate=%header{www-authenticate}\n' "$base/api/auth"

echo "== POST /api/auth（正确凭据）"
login="$(curl -sS -H 'Content-Type: application/json' \
  -d '{"username":"probe","password":"probe_pass_123"}' "$base/api/auth")"
token="$(printf '%s' "$login" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')"
echo "token 长度=${#token}"

echo "== GET /api/auth（带 token）"
curl -sS -o /dev/null -w 'status=%{http_code}\n' -H "Authorization: Bearer $token" "$base/api/auth"

echo "== 旧路径 /api/auth/login 与 /api/auth/session"
curl -sS -o /dev/null -w 'POST /api/auth/login -> %{http_code}\n' -X POST "$base/api/auth/login"
curl -sS -o /dev/null -w 'GET  /api/auth/session -> %{http_code}\n' "$base/api/auth/session"

# 3. 造一张 64×64 PNG 上传
cat >"$work/genpng.go" <<'EOF'
package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
)

func main() {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 4), G: uint8(y * 4), B: 0x40, A: 255})
		}
	}
	f, _ := os.Create(os.Args[1])
	defer f.Close()
	_ = png.Encode(f, img)
}
EOF
printf 'module pnggen\n\ngo 1.27.4\n' >"$work/genpng.mod"
(cd "$work" && mv genpng.mod go.mod && go run genpng.go "$work/skin.png")

echo "== POST /api/skins"
curl -sS -o /dev/null -w 'status=%{http_code}\n' -H "Authorization: Bearer $token" \
  -F "file=@$work/skin.png;type=image/png" -F kind=skin.steve -F tags=probe "$base/api/skins"

# 4. 超长 username 的失败登录（1 MiB 请求体上限内的 900 KiB）
python3 - "$work" <<'PY'
import json, sys
body = {"username": "a" * (900 * 1024), "password": "wrongpassword"}
open(sys.argv[1] + "/long.json", "w").write(json.dumps(body))
PY
echo "== POST /api/auth（900 KiB username，错误密码）"
/usr/bin/time -f 'elapsed=%es' curl -sS -o /dev/null -w 'status=%{http_code}\n' \
  -H 'Content-Type: application/json' --data-binary "@$work/long.json" "$base/api/auth" 2>&1 | tail -2

sleep 0.3
echo "== 审计与访问日志里的关键字段"
python3 - "$log" <<'PY'
import json, sys
for line in open(sys.argv[1]):
    line = line.strip()
    if not line:
        continue
    try:
        rec = json.loads(line)
    except json.JSONDecodeError:
        continue
    if rec.get("msg") == "audit":
        keys = sorted(k for k in rec if k not in ("timestamp", "level", "msg", "event", "client_ip"))
        extra = ""
        if rec.get("event") == "login":
            extra = f' username_len={len(rec.get("username", ""))}'
        print(f'audit event={rec.get("event")} fields={keys}{extra}')
    elif rec.get("msg") == "HTTP Request" and rec.get("path") in ("/api/auth", "/api/skins"):
        print(f'access path={rec["path"]} status={rec["status"]}')
PY
