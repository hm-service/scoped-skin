package httpx_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"scoped-skin/httpx"
)

// request 造一个带指定对端地址与头的请求。
func request(remoteAddr string, headers map[string]string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = remoteAddr
	for key, value := range headers {
		r.Header.Set(key, value)
	}
	return r
}

// DESIGN.md 5.5：对端不在可信网段里时，反代头一律不算数。
func TestClientIPIgnoresHeadersFromUntrustedPeer(t *testing.T) {
	t.Parallel()

	proxies, err := httpx.NewProxies("127.0.0.1/32")
	if err != nil {
		t.Fatalf("NewProxies: %v", err)
	}
	r := request("203.0.113.9:5555", map[string]string{
		"X-Forwarded-For": "10.0.0.1",
		"X-Real-IP":       "10.0.0.2",
	})
	if got := proxies.ClientIP(r); got != "203.0.113.9" {
		t.Fatalf("ClientIP = %q，想要 203.0.113.9", got)
	}
}

// 对端可信时从 X-Forwarded-For 右起取第一个不可信地址：反代追加在右边的那一跳伪造不了。
func TestClientIPWalksForwardedForFromTheRight(t *testing.T) {
	t.Parallel()

	proxies, err := httpx.NewProxies("127.0.0.1/32,10.0.0.0/8")
	if err != nil {
		t.Fatalf("NewProxies: %v", err)
	}

	// 客户端自己塞了一段假的 XFF，反代把真实地址追加在右边
	r := request("127.0.0.1:5555", map[string]string{
		"X-Forwarded-For": "9.9.9.9, 198.51.100.7, 10.0.0.5",
	})
	if got := proxies.ClientIP(r); got != "198.51.100.7" {
		t.Fatalf("ClientIP = %q，想要 198.51.100.7（左边那个 9.9.9.9 是伪造的）", got)
	}

	// 没有 XFF 时退回 X-Real-IP
	r = request("127.0.0.1:5555", map[string]string{"X-Real-IP": "198.51.100.8"})
	if got := proxies.ClientIP(r); got != "198.51.100.8" {
		t.Fatalf("ClientIP = %q，想要 198.51.100.8", got)
	}

	// 反代头全是垃圾/可信地址时，退回对端地址，不 panic
	r = request("127.0.0.1:5555", map[string]string{"X-Forwarded-For": "乱写, 10.0.0.9"})
	if got := proxies.ClientIP(r); got != "127.0.0.1" {
		t.Fatalf("ClientIP = %q，想要 127.0.0.1", got)
	}

	// IPv6 与端口写法
	ipv6, err := httpx.NewProxies("::1/128")
	if err != nil {
		t.Fatalf("NewProxies: %v", err)
	}
	r = request("[::1]:5555", map[string]string{"X-Forwarded-For": "2001:db8::1"})
	if got := ipv6.ClientIP(r); got != "2001:db8::1" {
		t.Fatalf("ClientIP = %q，想要 2001:db8::1", got)
	}
}

func TestNewProxiesDefaultsToLoopbackAndRejectsGarbage(t *testing.T) {
	t.Parallel()

	proxies, err := httpx.NewProxies("")
	if err != nil {
		t.Fatalf("空配置应当用默认值: %v", err)
	}
	r := request("127.0.0.1:5555", map[string]string{"X-Real-IP": "198.51.100.9"})
	if got := proxies.ClientIP(r); got != "198.51.100.9" {
		t.Fatalf("默认应当只信本机: ClientIP = %q", got)
	}

	if _, err := httpx.NewProxies("不是网段"); err == nil {
		t.Fatalf("非法配置应当报错")
	}
}
