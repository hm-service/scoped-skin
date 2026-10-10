package db

// User 是 DESIGN.md 4.1 的 user 表。
type User struct {
	UUID         string `json:"uuid"`
	Role         string `json:"role"`
	PlayerName   string `json:"player_name"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	Skin         string `json:"skin"`
	Cape         string `json:"cape"`
}

// Item 是一条贴图资源，字段与 DESIGN.md 3.6 的「贴图条目」一致。
//
// Kind 是库里那一列（skin.steve / skin.alex / cape，已含模型）；Category 是由它推出的
// 粗分类（skin / cape），前端分类与 Yggdrasil 的 SKIN / CAPE 分支都用它。
type Item struct {
	ID         string   `json:"id"`
	Kind       string   `json:"kind"`
	Category   string   `json:"category"`
	Tags       []string `json:"tags"`
	UploadAt   int64    `json:"upload_at"`
	Readonly   bool     `json:"readonly"`
	Width      int      `json:"width"`
	Height     int      `json:"height"`
	Size       int      `json:"size"`
	Favorite   bool     `json:"favorite"`
	TextureURL string   `json:"texture_url"`
}

type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

type Stats struct {
	Skins     int `json:"skins"`
	Capes     int `json:"capes"`
	Favorites int `json:"favorites"`
}

// builtinEntry 是 builtin/manifest.json 里的一项。
type builtinEntry struct {
	File string   `json:"file"`
	Kind string   `json:"kind"`
	Tags []string `json:"tags"`
}
