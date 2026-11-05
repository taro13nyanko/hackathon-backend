package model

import "time"

// Purchase は purchases テーブルの1行（決済はしない。買った記録だけ）。
type Purchase struct {
	Id        string    `json:"id"`
	Item      Item      `json:"item"`
	BuyerId   string    `json:"-"`
	Price     int       `json:"price"` // 買ったときの価格
	CreatedAt time.Time `json:"created_at"`
}

// PurchaseRes は POST /items/{id}/purchase のレスポンス。
type PurchaseRes struct {
	PurchaseId string `json:"purchase_id"`
	ItemId     string `json:"item_id"`
	Price      int    `json:"price"`
}
