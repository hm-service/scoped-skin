// Package httpx 收纳站点 API 与 Yggdrasil 两套 HTTP 层共用的响应工具。
package httpx

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// ContentType 是规范要求的 JSON 响应类型（Yggdrasil 规范：Content-Type 均为
// application/json; charset=utf-8）。
const ContentType = "application/json; charset=utf-8"

// Error 是站点 /api/* 的错误体；前端读的是 detail 字段。
type Error struct {
	Detail string `json:"detail"`
}

func Write(w http.ResponseWriter, status int, payload any) {
	body, err := json.Marshal(payload)
	if err != nil {
		slog.Error("json encode failed", slog.Any("error", err))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", ContentType)
	w.WriteHeader(status)
	w.Write(body)
}

func Fail(w http.ResponseWriter, status int, detail string) {
	Write(w, status, Error{Detail: detail})
}

func Unauthorized(w http.ResponseWriter, detail string) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	Fail(w, http.StatusUnauthorized, detail)
}

// Proxies 决定客户端 IP 从哪里取（DESIGN.md 5.5）：只有连接的对端落在可信网段里
// （也就是请求确实来自我们自己的反向代理）时才认反代头，否则一律用对端地址。
// 这样即使服务被直接访问、有人自己伪造 X-Forwarded-For，也拿不到「别人」的身份。
type Proxies struct {
	trusted []netip.Prefix
}

// DefaultTrustedProxy 是 TRUSTED_PROXIES 的默认值：只信本机（同机反代即可开箱可用）。
const DefaultTrustedProxy = "127.0.0.1/32,::1/128"

// NewProxies 解析逗号分隔的 IP / CIDR 列表；空串按 DefaultTrustedProxy 处理。
func NewProxies(spec string) (*Proxies, error) {
	if strings.TrimSpace(spec) == "" {
		spec = DefaultTrustedProxy
	}
	proxies := &Proxies{}
	for _, item := range strings.Split(spec, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if prefix, err := netip.ParsePrefix(item); err == nil {
			proxies.trusted = append(proxies.trusted, prefix.Masked())
			continue
		}
		addr, err := netip.ParseAddr(item)
		if err != nil {
			return nil, fmt.Errorf("trusted proxy %q is neither an IP nor a CIDR", item)
		}
		proxies.trusted = append(proxies.trusted, netip.PrefixFrom(addr, addr.BitLen()))
	}
	return proxies, nil
}

// 对端不可信 → 直接用对端地址。对端可信 → 从 X-Forwarded-For 的**最右**开始往左走，
// 跳过可信网段里的每一跳，第一个不可信的就是真正的客户端：反代用
// `$proxy_add_x_forwarded_for` 时会把客户端自己伪造的部分拼在左边、把真实来源追加在右边，
// 所以右起第一个不可信地址是伪造不了的。没有 XFF 时退回 X-Real-IP。
func (p *Proxies) ClientIP(r *http.Request) string {
	peer := peerIP(r.RemoteAddr)
	peerAddr, err := netip.ParseAddr(peer)
	if p == nil || err != nil || !p.isTrusted(peerAddr) {
		return peer
	}
	for _, item := range reverse(splitForwarded(r.Header.Get("X-Forwarded-For"))) {
		addr, err := netip.ParseAddr(strings.TrimSpace(item))
		if err != nil {
			continue
		}
		if !p.isTrusted(addr) {
			return addr.Unmap().String()
		}
	}
	if addr, err := netip.ParseAddr(strings.TrimSpace(r.Header.Get("X-Real-IP"))); err == nil {
		return addr.Unmap().String()
	}
	return peer
}

func (p *Proxies) isTrusted(addr netip.Addr) bool {
	if !addr.IsValid() {
		return false
	}
	addr = addr.Unmap()
	for _, prefix := range p.trusted {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

// peerIP 取 RemoteAddr 里的主机部分；解析不出来就原样返回。
func peerIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	addr, err := netip.ParseAddr(strings.Trim(host, "[]"))
	if err != nil {
		return host
	}
	return addr.Unmap().String()
}

func splitForwarded(header string) []string {
	if strings.TrimSpace(header) == "" {
		return nil
	}
	return strings.Split(header, ",")
}

func reverse(values []string) []string {
	out := make([]string, 0, len(values))
	for index := len(values) - 1; index >= 0; index-- {
		out = append(out, values[index])
	}
	return out
}
