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
	CreatedAt   time.Time `json:"-"`
}

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
