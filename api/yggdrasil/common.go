// Package yggdrasil 实现 authlib-injector 的《Yggdrasil 服务端技术规范》。
//
//	https://yushijinhun.github.io/authlib-injector/zh/Yggdrasil-服务端技术规范.html
//
// 端点挂在 /api/yggdrasil（DESIGN.md 3.5），accessToken / clientToken 都是 32 位
// 无横线 hex、只活在内存里（30 天过期，进程重启即失效）；贴图属性用 RSA-SHA1 签名。
package yggdrasil

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"scoped-skin/app"
	"scoped-skin/httpx"
)

// Server 把应用容器接到 Yggdrasil 端点上。
type Server struct {
	App *app.App
}

// New 造一个 Yggdrasil 服务。
func New(instance *app.App) *Server { return &Server{App: instance} }

// ErrorResponse 是规范里的错误体。一般 HTTP 异常只填 Error（取状态码的 Reason Phrase），
// 业务异常两条都填。
type ErrorResponse struct {
	Error        string `json:"error"`
	ErrorMessage string `json:"errorMessage,omitempty"`
	Cause        string `json:"cause,omitempty"`
}

// 规范「错误信息格式」里出现最多的两条。
const (
	// 令牌无效 / 试图使用一个错误的角色加入服务器
	errInvalidToken = "Invalid token."
	// 密码错误，或短时间内多次登录失败而被暂时禁止登录
	errInvalidCredentials = "Invalid credentials. Invalid username or password."
)

func sendError(w http.ResponseWriter, status int, kind, message string) {
	httpx.Write(w, status, ErrorResponse{Error: kind, ErrorMessage: message})
}

// forbidden 是规范里业务异常的固定写法：403 + ForbiddenOperationException。
func forbidden(w http.ResponseWriter, message string) {
	sendError(w, http.StatusForbidden, "ForbiddenOperationException", message)
}

// badRequest 走规范给「一般 HTTP 异常」留的口子：error 取该 HTTP 状态对应的
// Reason Phrase，errorMessage 不定义。methodNotAllowed / notFound 同款，只是
// 它们要当 handler 用，所以签名照 http.HandlerFunc 来。
func badRequest(w http.ResponseWriter, message string) {
	sendError(w, http.StatusBadRequest, "Bad Request", message)
}

func methodNotAllowed(w http.ResponseWriter, r *http.Request) {
	sendError(w, http.StatusMethodNotAllowed, "Method Not Allowed", "")
}

func notFound(w http.ResponseWriter, r *http.Request) {
	sendError(w, http.StatusNotFound, "Not Found", "")
}

func internalError(w http.ResponseWriter, err error) {
	slog.Error("Yggdrasil 内部错误", slog.Any("error", err))
	sendError(w, http.StatusInternalServerError, "Internal Server Error", "")
}

// decode 解析 JSON 请求体；失败时自己回 400 并返回 false。
func decode[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	var payload T
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&payload); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			badRequest(w, "Request body too large")
		} else {
			badRequest(w, "Malformed JSON body")
		}
		return payload, false
	}
	return payload, true
}
