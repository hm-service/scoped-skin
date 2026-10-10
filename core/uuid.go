package core

import (
	"crypto/md5"
	"encoding/hex"
	"strings"
)

// OfflineUUID 按 Yggdrasil 规范的「兼容离线验证」一节生成角色 UUID：
//
//	UUID.nameUUIDFromBytes(("OfflinePlayer:" + characterName).getBytes(UTF_8))
//
// 即 md5 摘要打上 version 3 标记，输出 32 位无横线小写 hex。用同一套规则推导，
// 玩家从离线验证切过来时 UUID 不变，存档不会丢。
func OfflineUUID(playerName string) string {
	sum := md5.Sum([]byte("OfflinePlayer:" + playerName))
	sum[6] = (sum[6] & 0x0f) | 0x30 // version 3
	sum[8] = (sum[8] & 0x3f) | 0x80 // RFC 4122 variant
	return hex.EncodeToString(sum[:])
}

// DashlessUUID 去掉 UUID 里的横线并转小写；Yggdrasil 规范里凡说「无符号 UUID」
// 的地方都要先过一道。
func DashlessUUID(value string) string {
	return strings.ToLower(strings.ReplaceAll(value, "-", ""))
}
