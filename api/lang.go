package api

import (
	"context"
	"errors"
	"net/http"

	"scoped-skin/core"
	"scoped-skin/httpx"
	"scoped-skin/i18n"
)

type langKey struct{}

// WithLanguage 按 Accept-Language 挑一档文案语言放进请求上下文，并回 Content-Language。
// Vary 是提醒缓存：同一个 URL 换语言就是另一份响应。
func WithLanguage(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lang := i18n.Parse(r.Header.Get("Accept-Language"))
		w.Header().Set("Content-Language", string(lang))
		w.Header().Add("Vary", "Accept-Language")
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), langKey{}, lang)))
	})
}

// language 取当前请求的文案语言；没经过中间件时就是源语言（简体中文）。
func language(r *http.Request) i18n.Lang {
	if lang, ok := r.Context().Value(langKey{}).(i18n.Lang); ok {
		return lang
	}
	return i18n.Default
}

// fail 写站点 API 的错误体 {"detail": "..."}，文案按请求语言取。
func fail(w http.ResponseWriter, r *http.Request, status int, key string, args ...any) {
	httpx.Fail(w, status, i18n.T(language(r), key, args...))
}

// unauthorized 是 401 的固定写法（带 WWW-Authenticate），文案同样本地化。
func unauthorized(w http.ResponseWriter, r *http.Request, key string, args ...any) {
	httpx.Unauthorized(w, i18n.T(language(r), key, args...))
}

// businessMessage 把 core / db 抛的带键错误渲染成当前语言的文案；不是这两种错误就返回空串。
func businessMessage(r *http.Request, err error) string {
	lang := language(r)
	if conflict, ok := errors.AsType[*core.ConflictError](err); ok {
		return i18n.T(lang, conflict.Key, conflict.Args...)
	}
	if invalid, ok := errors.AsType[*core.ValidationError](err); ok {
		return i18n.T(lang, invalid.Key, invalid.Args...)
	}
	return ""
}

// isConflict 判断是不是撞唯一约束（撞了回 409，其余字段问题回 400）。
func isConflict(err error) bool {
	var conflict *core.ConflictError
	return errors.As(err, &conflict)
}
