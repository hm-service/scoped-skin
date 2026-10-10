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
type Item struct {
	ID         string   `json:"id"`
	Kind       string   `json:"kind"`
	Model      string   `json:"model"`
	Tags       []string `json:"tags"`
	UploadAt   int64    `json:"upload_at"`
	Builtin    bool     `json:"is_builtin"`
	Width      int      `json:"width"`
	Height     int      `json:"height"`
	Size       int      `json:"size"`
	Favorite   bool     `json:"favorite"`
	TextureURL string   `json:"texture_url"`
}

// TagCount 是一个标签及其使用次数。
type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

// Stats 是库里的统计数字，给前端做角标。
type Stats struct {
	Skins     int `json:"skins"`
	Capes     int `json:"capes"`
	Favorites int `json:"favorites"`
}

// builtinEntry 是 builtin/manifest.json 里的一项。
type builtinEntry struct {
	File     string   `json:"file"`
	Category string   `json:"category"`
	Model    string   `json:"model"`
	Tags     []string `json:"tags"`
}
