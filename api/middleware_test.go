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

// DESIGN.md 5.3：/api/* 共用 128 个并发槽位，超出的排队。
func TestConcurrencyCapsInFlightRequests(t *testing.T) {
	t.Parallel()

	var inFlight, peak atomic.Int64
	handler := api.Concurrency(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	total := api.MaxConcurrentRequests + 32
	var group sync.WaitGroup
	for range total {
		group.Go(func() {
			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/library", nil))
		})
	}
	group.Wait()

	if got := peak.Load(); got > api.MaxConcurrentRequests {
		t.Fatalf("同时在处理的请求峰值 = %d，超过上限 %d", got, api.MaxConcurrentRequests)
	} else if got < 2 {
		t.Fatalf("峰值只有 %d，并发没跑起来，这个用例说明不了问题", got)
	}
}

// 排队时客户端断开：直接放行，不占槽位也不调用后面的处理器。
func TestConcurrencyGivesUpWhenClientGoesAway(t *testing.T) {
	t.Parallel()

	block := make(chan struct{})
	release := make(chan struct{}, api.MaxConcurrentRequests)
	handler := api.Concurrency(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		release <- struct{}{}
		<-block
	}))

	for range api.MaxConcurrentRequests {
		go handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/library", nil))
	}
	for range api.MaxConcurrentRequests {
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

// 页面与静态资源不占槽位：槽位被 /api/* 占满时它们照样立刻过。
func TestConcurrencySkipsStaticPaths(t *testing.T) {
	t.Parallel()

	block := make(chan struct{})
	release := make(chan struct{}, api.MaxConcurrentRequests)
	handler := api.Concurrency(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			return // 非 /api/* 的请求本来就该直接过
		}
		release <- struct{}{}
		<-block
	}))

	for range api.MaxConcurrentRequests {
		go handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/library", nil))
	}
	for range api.MaxConcurrentRequests {
		<-release
	}

	for _, path := range []string{"/", "/assets/app-abc.js", "/asset/abc", "/livez"} {
		done := make(chan struct{})
		go func() {
			defer close(done)
			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("%s 不该被并发闸门挡住（槽位只留给 /api/*）", path)
		}
	}
	close(block)
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
