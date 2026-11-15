package model

import (
	"encoding/json"
	"time"
)

// 商品の状態。
const (
	ItemOnSale = "on_sale" // 出品中
	ItemSold   = "sold"    // 売り切れ（購入機能で使う）
)

// 出品の確認（禁止物の検知）の状態（item.moderation_status、sql/008_ai_features.sql）。
const (
	ModerationOK     = "ok"     // 問題なし（既存の出品・見本データもこれ）
	ModerationReview = "review" // 要確認（公開はされる。管理者が確認する）
	ModerationHidden = "hidden" // 非公開（管理者が決める。一覧に出さず、出品者と管理者だけが見られる）
)

// Item は item テーブルの1行（出品された商品）。
type Item struct {
	Id          string  `json:"id"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Price       int     `json:"price"`
	Status      string  `json:"status"`
	Seller      *Seller `json:"seller"` // 見本データは null
	// 写真と、Gemini が作る模型の絵（images テーブルの ID。無ければ空）。画像は GET /images/{id}
	PhotoImageId  string `json:"photo_image_id"`
	FigureImageId string `json:"figure_image_id"`
	FigureCaption string `json:"figure_caption"`
	// 保存された模型の設計図（POST /figures が返す形の JSON）。無ければ null。PUT /items/{id}/figure で保存する
	Figure    json.RawMessage `json:"figure"`
	CreatedAt time.Time       `json:"created_at"`
	// 出品の確認の状態（ok / review / hidden）はだれにでも返す（画面の「確認中」の印）。
	// 理由は出品者本人と管理者のときだけ入れ、ほかの人には空文字にする（usecase が消す）
	ModerationStatus string `json:"moderation_status"`
	ModerationReason string `json:"moderation_reason"`
	// いいねの数と、ログイン中の本人がいいね済みか（未ログインなら false）
	LikeCount int  `json:"like_count"`
	Liked     bool `json:"liked"`
}

// IsHidden は管理者が非公開にした商品か。
func (it Item) IsHidden() bool { return it.ModerationStatus == ModerationHidden }

// ItemQuery は商品一覧（GET /items）の条件。
type ItemQuery struct {
	Keyword  string // タイトルか説明文に含む（空なら全件）
	MinPrice int    // この値段以上（0 なら指定なし）
	MaxPrice int    // この値段以下（0 なら指定なし）
	ViewerId string // liked を計算する本人の users.id（未ログインなら空）
}

// ItemCreateReq は POST /items のリクエストボディ。
// 出品者はボディではなく、ログイン情報（ID トークン）から付ける。
type ItemCreateReq struct {
	Title        string `json:"title"`
	Description  string `json:"description"`
	Price        int    `json:"price"`
	PhotoImageId string `json:"photo_image_id"` // 任意。POST /images で上げた自分の写真
}

// ItemCreateRes は POST /items のレスポンス（出品の確認の結果つき。出品者本人なので理由も返す）。
type ItemCreateRes struct {
	Id               string `json:"id"`
	ModerationStatus string `json:"moderation_status"`
	ModerationReason string `json:"moderation_reason"`
}

// ModerationUpdateReq は PUT /admin/items/{id}/moderation のリクエストボディ。
type ModerationUpdateReq struct {
	Status string `json:"status"` // ok（公開に戻す）か hidden（非公開）
	Note   string `json:"note"`   // 任意。あれば moderation_reason をこれで上書きする
}

// LikeRes は PUT/DELETE /items/{id}/like のレスポンス。
type LikeRes struct {
	Liked     bool `json:"liked"`
	LikeCount int  `json:"like_count"`
}
