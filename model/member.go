package model

import (
	"errors"
	"time"
)

// Member は users テーブルの1行（Google でログインしたアプリの利用者）。
// カリキュラムの user テーブル（model.User）と混ざらないよう、Go では Member と呼ぶ。
type Member struct {
	Id          string    `json:"id"`
	FirebaseUID string    `json:"-"` // 外には出さない
	DisplayName string    `json:"display_name"`
	Role        string    `json:"role"` // member（既定）か admin（管理者。DB で直接 UPDATE して決める）
	CreatedAt   time.Time `json:"-"`
}

// 利用者のロール（users.role、sql/008_ai_features.sql）。
const (
	RoleMember = "member"
	RoleAdmin  = "admin"
)

// IsAdmin は管理者か（要確認・非公開の商品を見て、公開・非公開を決められる）。
func (m Member) IsAdmin() bool { return m.Role == RoleAdmin }

// MemberCreateReq は POST /me のリクエストボディ。
type MemberCreateReq struct {
	DisplayName string `json:"display_name"`
}

// Seller は商品に付ける出品者の情報（見本データは出品者なし＝null）。
type Seller struct {
	Id          string `json:"id"`
	DisplayName string `json:"display_name"`
}

// ErrAlreadyRegistered はプロフィール登録済みなのに、もう一度登録しようとしたとき（→ 409）。
var ErrAlreadyRegistered = errors.New("already registered")
