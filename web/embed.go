// Package web 把前端静态资源打进二进制：单文件部署，不依赖工作目录。
package web

import "embed"

// Files 是站点页面、脚本、样式与第三方库（Vue 3 全局构建 + skinview3d）。
// 页面用 *.html 通配，新加一个页面不用回来改这行。
//
//go:embed *.html css js vendor
var Files embed.FS
