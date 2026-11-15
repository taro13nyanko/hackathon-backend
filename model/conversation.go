package model

import "time"

// Conversation は1つの商品について、買いたい人と出品者の2人のやりとり（conversations テーブル）。
type Conversation struct {
	Id          string    `json:"id"`
	Item        Item      `json:"item"`
	Partner     Seller    `json:"partner"`      // 相手（自分が買い手なら出品者、出品者なら買い手）
	LastMessage *Message  `json:"last_message"` // まだ無ければ null
	UnreadCount int       `json:"unread_count"` // 相手から来た、既読の位置より新しいメッセージの数
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

// ReadReq は PUT /conversations/{id}/read のリクエストボディ（省略可）。
// LastMessageId を渡すと、そのメッセージまで（それより新しいものは未読のまま）を既読にする。空なら最新まで。
type ReadReq struct {
	LastMessageId string `json:"last_message_id"`
}

// ReadRes は PUT /conversations/{id}/read の返り値。
type ReadRes struct {
	LastReadMessageId string `json:"last_read_message_id"` // 既読の位置（メッセージがまだ無ければ空）
	UnreadCount       int    `json:"unread_count"`         // 既読にした後の、自分の未読の合計（ナビの数）
}

// UnreadRes は GET /me/unread の返り値。
type UnreadRes struct {
	Count int `json:"count"` // 自分のやりとり全部の未読の合計
}
