package model

import "time"

// 商品の状態。
const (
	ItemOnSale = "on_sale" // 出品中
	ItemSold   = "sold"    // 売り切れ（購入機能で使う）
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
	PhotoImageId  string    `json:"photo_image_id"`
	FigureImageId string    `json:"figure_image_id"`
	FigureCaption string    `json:"figure_caption"`
	CreatedAt     time.Time `json:"created_at"`
}

// ItemCreateReq は POST /items のリクエストボディ。
// 出品者はボディではなく、ログイン情報（ID トークン）から付ける。
type ItemCreateReq struct {
	Title        string `json:"title"`
	Description  string `json:"description"`
	Price        int    `json:"price"`
	PhotoImageId string `json:"photo_image_id"` // 任意。POST /images で上げた自分の写真
}

// ItemCreateRes は POST /items のレスポンス。
type ItemCreateRes struct {
	Id string `json:"id"`
}
