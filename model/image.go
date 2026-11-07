package model

import "time"

// Image は images テーブルの1行（商品の写真や、Gemini が作った模型の絵）。
type Image struct {
	Id          string
	OwnerId     string // 上げた人（users.id）
	ContentType string // image/jpeg など
	Data        []byte
	CreatedAt   time.Time
}
