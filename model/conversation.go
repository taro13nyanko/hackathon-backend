package model

import "time"

// Conversation は1つの商品について、買いたい人と出品者の2人のやりとり（conversations テーブル）。
type Conversation struct {
	Id          string    `json:"id"`
	Item        Item      `json:"item"`
	Partner     Seller    `json:"partner"`      // 相手（自分が買い手なら出品者、出品者なら買い手）
	LastMessage *Message  `json:"last_message"` // まだ無ければ null
	BuyerId     string    `json:"-"`
	CreatedAt   time.Time `json:"-"`
}

// Message は messages テーブルの1行。
type Message struct {
	Id        string    `json:"id"` // ULID なので ID の順＝送った順
	SenderId  string    `json:"sender_id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// MessageCreateReq は POST /conversations/{id}/messages のリクエストボディ。
type MessageCreateReq struct {
	Body string `json:"body"`
}
