package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"scoped-skin/api"
)

// DESIGN.md 5.3：闸门把同时在处理的请求卡在槽位数以内，超出的排队。
func TestConcurrencyCapsInFlightRequests(t *testing.T) {
	t.Parallel()

	var inFlight, peak atomic.Int64
	handler := api.Concurrency(api.DefaultAPIConcurrency, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := inFlight.Add(1)
		for {
			previous := peak.Load()
			if current <= previous || peak.CompareAndSwap(previous, current) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		inFlight.Add(-1)
	}))

	total := api.DefaultAPIConcurrency + 32
	var group sync.WaitGroup
	for range total {
		group.Go(func() {
			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/library", nil))
		})
	}
	group.Wait()

	if got := peak.Load(); got > api.DefaultAPIConcurrency {
		t.Fatalf("同时在处理的请求峰值 = %d，超过上限 %d", got, api.DefaultAPIConcurrency)
	} else if got < 2 {
		t.Fatalf("峰值只有 %d，并发没跑起来，这个用例说明不了问题", got)
	}
}

// 排队时客户端断开：直接放行，不占槽位也不调用后面的处理器。
func TestConcurrencyGivesUpWhenClientGoesAway(t *testing.T) {
	t.Parallel()

	block := make(chan struct{})
	release := make(chan struct{}, api.DefaultAPIConcurrency)
	handler := api.Concurrency(api.DefaultAPIConcurrency, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		release <- struct{}{}
		<-block
	}))

	for range api.DefaultAPIConcurrency {
		go handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/library", nil))
	}
	for range api.DefaultAPIConcurrency {
		<-release
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := httptest.NewRequest(http.MethodGet, "/api/library", nil).WithContext(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.ServeHTTP(httptest.NewRecorder(), request)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("客户端断开后排队请求应当立刻放行")
	}
	close(block)
}

// 闸门不认路径：套在谁身上就卡谁（API 与静态各套一层是 main.go 的事）。
// 这里用一条以前会被跳过的路径（"/"）证明它同样要排队。
func TestConcurrencyCountsEveryPathItWraps(t *testing.T) {
	t.Parallel()

	block := make(chan struct{})
	// 两个请求都会走进处理函数，信号通道留够容量，免得第二个卡在发信号这一步
	entered := make(chan struct{}, 2)
	handler := api.Concurrency(1, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-block
	}))

	go handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	<-entered

	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/assets/app-abc.js", nil))
	}()
	select {
	case <-done:
		t.Fatal("槽位被占着，第二个请求不该过")
	case <-time.After(100 * time.Millisecond):
	}

	close(block)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("槽位空出来后排队请求应当立刻过")
	}
}

// 访问日志不记纯静态与存活探针（噪音），但接口照记。页面走的是同一层中间件，
// 只是由站点外壳那边测（根目录 web_test.go）。
func TestAccessLogSkipsStaticAndLivez(t *testing.T) {
	_, handler := newSite(t)
	read := captureLogs(t)

	silent := []string{"/assets/app-abc.js", "/asset/" + strings.Repeat("a", 64), "/livez"}
	for _, path := range silent {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}
	noisy := []string{"/api/nope", "/readyz"}
	for _, path := range noisy {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}

	logged := map[string]bool{}
	for _, record := range read() {
		if record["msg"] != "HTTP Request" {
			continue
		}
		if path, ok := record["path"].(string); ok {
			logged[path] = true
		}
	}
	for _, path := range silent {
		if logged[path] {
			t.Errorf("%s 不该写访问日志", path)
		}
	}
	for _, path := range noisy {
		if !logged[path] {
			t.Errorf("%s 应当写访问日志", path)
		}
	}
}
