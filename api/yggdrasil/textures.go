package yggdrasil

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"

	"scoped-skin/core"
	"scoped-skin/db"
)

// modelMetadata 把库里的 model_type 映射成规范里的材质模型：
// steve → default（正常手臂宽度）、alex → slim（细手臂）；
// share 表示「交给客户端自己判断」，所以不带 metadata。
var modelMetadata = map[string]string{
	"steve": "default",
	"alex":  "slim",
}

// 规范里材质信息的结构；把 texturePayload 整个 base64 之后就是 textures 属性的值。
type textureMetadata struct {
	Model string `json:"model"`
}

type texture struct {
	URL      string           `json:"url"`
	Metadata *textureMetadata `json:"metadata,omitempty"`
}

type texturePayload struct {
	Timestamp   int64              `json:"timestamp"`
	ProfileID   string             `json:"profileId"`
	ProfileName string             `json:"profileName"`
	Textures    map[string]texture `json:"textures"`
}

// property 是角色属性；signature 是属性值的数字签名（SHA1withRSA，Base64）。
type property struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	Signature string `json:"signature,omitempty"`
}

// profile 是「角色信息的序列化」。
type profile struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Properties []property `json:"properties,omitempty"`
}

// userInfo 是「用户信息的序列化」；本站每个账号就一个角色，ID 与角色 UUID 相同。
type userInfo struct {
	ID         string     `json:"id"`
	Properties []property `json:"properties"`
}

// profileOf 只带 id / name：可用角色列表与绑定角色用这一份，不带属性。
func profileOf(user *db.User) profile {
	return profile{ID: core.DashlessUUID(user.UUID), Name: user.PlayerName}
}

func userInfoOf(user *db.User) *userInfo {
	return &userInfo{ID: core.DashlessUUID(user.UUID), Properties: []property{}}
}

// texturesProperty 组装 textures 属性；signed 为真时用 RSA-SHA1 给它签名。
//
// 规范：签名算法为 SHA1withRSA，签的对象就是属性值（那串 base64）本身。
func (s *Server) texturesProperty(r *http.Request, user *db.User, signed bool) (property, error) {
	payload := texturePayload{
		Timestamp:   time.Now().UnixMilli(), // Java 时间戳：UTC 毫秒
		ProfileID:   core.DashlessUUID(user.UUID),
		ProfileName: user.PlayerName,
		Textures:    map[string]texture{},
	}
	if user.Skin != "" {
		item, err := s.App.Store.Item(user.Skin, user.UUID)
		if err != nil {
			return property{}, err
		}
		if item != nil && item.Kind == "skin" {
			entry := texture{URL: assetURL(r, item.ID)}
			if model, ok := modelMetadata[item.Model]; ok {
				entry.Metadata = &textureMetadata{Model: model}
			}
			payload.Textures["SKIN"] = entry
		}
	}
	if user.Cape != "" {
		item, err := s.App.Store.Item(user.Cape, user.UUID)
		if err != nil {
			return property{}, err
		}
		if item != nil && item.Kind == "cape" {
			payload.Textures["CAPE"] = texture{URL: assetURL(r, item.ID)}
		}
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return property{}, err
	}
	value := base64.StdEncoding.EncodeToString(raw)
	result := property{Name: "textures", Value: value}
	if signed {
		signature, err := s.App.Keys.Sign(value)
		if err != nil {
			return property{}, err
		}
		result.Signature = signature
	}
	return result, nil
}

// fullProfile 是带属性（以及可选签名）的角色信息，hasJoined 与 profile 都用它。
func (s *Server) fullProfile(r *http.Request, user *db.User, signed bool) (profile, error) {
	entry, err := s.texturesProperty(r, user, signed)
	if err != nil {
		return profile{}, err
	}
	return profile{
		ID:         core.DashlessUUID(user.UUID),
		Name:       user.PlayerName,
		Properties: []property{entry},
	}, nil
}
