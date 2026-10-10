package httpx_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"scoped-skin/httpx"
)

func TestWriteFailAndUnauthorized(t *testing.T) {
	recorder := httptest.NewRecorder()
	httpx.Write(recorder, http.StatusTeapot, httpx.Error{Detail: "hi"})
	if recorder.Code != http.StatusTeapot {
		t.Errorf("状态码 = %d，想要 %d", recorder.Code, http.StatusTeapot)
	}
	if got := recorder.Header().Get("Content-Type"); got != httpx.ContentType {
		t.Errorf("Content-Type = %q，想要 %q", got, httpx.ContentType)
	}
	if got := recorder.Body.String(); got != `{"detail":"hi"}` {
		t.Errorf("响应体 = %q", got)
	}

	recorder = httptest.NewRecorder()
	httpx.Fail(recorder, http.StatusBadRequest, "坏了")
	if recorder.Code != http.StatusBadRequest || recorder.Body.String() != `{"detail":"坏了"}` {
		t.Errorf("Fail = %d %q", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	httpx.Unauthorized(recorder, "Not signed in")
	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("Unauthorized 状态码 = %d，想要 401", recorder.Code)
	}
	if got := recorder.Header().Get("WWW-Authenticate"); got != "Bearer" {
		t.Errorf("WWW-Authenticate = %q，想要 Bearer", got)
	}
	if got := recorder.Body.String(); got != `{"detail":"Not signed in"}` {
		t.Errorf("Unauthorized 响应体 = %q", got)
	}
}

// 序列化不了的值必须回 500 纯文本兜底，而不是把半截 JSON 写出去。
func TestWriteFallsBackWhenMarshalFails(t *testing.T) {
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(discard{}, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	recorder := httptest.NewRecorder()
	httpx.Write(recorder, http.StatusOK, map[string]any{"bad": make(chan int)})
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("序列化失败时状态码 = %d，想要 500（%s）", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); got == httpx.ContentType {
		t.Errorf("序列化失败时不应当是 JSON Content-Type: %q", got)
	}
	if !strings.Contains(recorder.Body.String(), "Internal Server Error") {
		t.Errorf("兜底响应体 = %q", recorder.Body.String())
	}
}

// 空白项跳过、裸 IP（没有 /32）也当单地址处理。
func TestNewProxiesSkipsBlanksAndAcceptsBareIP(t *testing.T) {
	proxies, err := httpx.NewProxies(" 127.0.0.1/32 , ,10.0.0.1,  ")
	if err != nil {
		t.Fatalf("NewProxies: %v", err)
	}
	r := request("10.0.0.1:5555", map[string]string{"X-Real-IP": "198.51.100.11"})
	if got := proxies.ClientIP(r); got != "198.51.100.11" {
		t.Errorf("裸 IP 应当被当成可信代理: ClientIP = %q", got)
	}
}

// 对端地址各种写法都要能解析出来；解析不了就原样回，不 panic。
// 反代头里全是可信地址时退回 X-Real-IP；X-Real-IP 也非法就退回对端。
func TestClientIPPeerParsingEdges(t *testing.T) {
	proxies, err := httpx.NewProxies("127.0.0.1/32,10.0.0.0/8")
	if err != nil {
		t.Fatalf("NewProxies: %v", err)
	}

	cases := []struct {
		name       string
		remoteAddr string
		headers    map[string]string
		want       string
	}{
		{"对端没有端口", "192.0.2.9", nil, "192.0.2.9"},
		{"对端是主机名", "weird-host:1234", nil, "weird-host"},
		{"对端完全解析不了", "not an addr", nil, "not an addr"},
		{"IPv4 映射的 IPv6 回环", "[::ffff:127.0.0.1]:5555",
			map[string]string{"X-Forwarded-For": "198.51.100.5"}, "198.51.100.5"},
		{"XFF 带空格和空项", "127.0.0.1:5555",
			map[string]string{"X-Forwarded-For": ", 198.51.100.6 , "}, "198.51.100.6"},
		{"XFF 全是可信地址 → 看 X-Real-IP", "127.0.0.1:5555",
			map[string]string{"X-Forwarded-For": "10.0.0.9", "X-Real-IP": "198.51.100.7"}, "198.51.100.7"},
		{"X-Real-IP 非法 → 回对端", "127.0.0.1:5555",
			map[string]string{"X-Real-IP": "不是地址"}, "127.0.0.1"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			r := request(testCase.remoteAddr, testCase.headers)
			if got := proxies.ClientIP(r); got != testCase.want {
				t.Errorf("ClientIP = %q，想要 %q", got, testCase.want)
			}
		})
	}

	// nil 接收者不该 panic（默认没有可信代理，直接用对端）
	var nilProxies *httpx.Proxies
	if got := nilProxies.ClientIP(request("203.0.113.9:5555", map[string]string{
		"X-Forwarded-For": "198.51.100.7",
	})); got != "203.0.113.9" {
		t.Errorf("nil Proxies 的 ClientIP = %q，想要对端地址", got)
	}
}

// discard 是给 slog 用的黑洞写入器，免得序列化失败的日志污染测试输出。
type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }
