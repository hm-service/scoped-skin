// Package builtin 把随包发行的官方默认皮肤（manifest.json + skins/*.png）打进二进制。
package builtin

import "embed"

// Files 是内置资源的清单与图片；manifest.json 在根，图片在 skins/ 下。
//
//go:embed manifest.json skins
var Files embed.FS
