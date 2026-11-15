package dao

import (
	"database/sql"
	"fmt"
	"time"

	"hackathon-backend/model"
	"hackathon-backend/usecase"
)

// LikeDao はいいね（likes テーブル、sql/008_ai_features.sql）。
type LikeDao struct {
	DB *sql.DB
}

var _ usecase.LikeStore = (*LikeDao)(nil)

// Like はいいねを付ける。もう付いていれば何もしない（主キー (user_id, item_id) の重複は成功扱い）。
func (d *LikeDao) Like(userId, itemId string, at time.Time) error {
	_, err := d.DB.Exec("INSERT INTO likes (user_id, item_id, created_at) VALUES (?, ?, ?)", userId, itemId, at)
	if err != nil && !isDuplicate(err) {
		return fmt.Errorf("insert like: %w", err)
	}
	return nil
}

// Unlike はいいねを外す。付いていなくても成功。
func (d *LikeDao) Unlike(userId, itemId string) error {
	if _, err := d.DB.Exec("DELETE FROM likes WHERE user_id = ? AND item_id = ?", userId, itemId); err != nil {
		return fmt.Errorf("delete like: %w", err)
	}
	return nil
}

// CountLikes は商品のいいねの数（idx_likes_item を使う）。
func (d *LikeDao) CountLikes(itemId string) (int, error) {
	var n int
	if err := d.DB.QueryRow("SELECT COUNT(*) FROM likes WHERE item_id = ?", itemId).Scan(&n); err != nil {
		return 0, fmt.Errorf("count likes: %w", err)
	}
	return n, nil
}

// LikedListMax は自分のいいね一覧で返す件数の上限。
const LikedListMax = 100

// FindLikedBy は userId がいいねした商品を、いいねした新しい順に返す（hidden は除く。idx_likes_user_created を使う）。
func (d *LikeDao) FindLikedBy(userId string) ([]model.Item, error) {
	items := &ItemDao{DB: d.DB}
	return items.queryItems(itemSelect+" JOIN likes lk ON lk.item_id = i.id"+
		" WHERE lk.user_id = ? AND "+itemPublic+
		" ORDER BY lk.created_at DESC, i.id DESC LIMIT ?", userId, userId, LikedListMax)
}
