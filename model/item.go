package model

import "time"

// 商品の状態。
const (
	ItemOnSale = "on_sale" // 出品中
	ItemSold   = "sold"    // 売り切れ（購入機能で使う）
)

// Item は item テーブルの1行（出品された商品）。
type Item struct {
	Id          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Price       int       `json:"price"`
	Status      string    `json:"status"`
	Seller      *Seller   `json:"seller"` // 見本データは null
	CreatedAt   time.Time `json:"created_at"`
}

// ItemCreateReq は POST /items のリクエストボディ。
// 出品者はボディではなく、ログイン情報（ID トークン）から付ける。
type ItemCreateReq struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Price       int    `json:"price"`
}

// ItemCreateRes は POST /items のレスポンス。
type ItemCreateRes struct {
	Id string `json:"id"`
}
