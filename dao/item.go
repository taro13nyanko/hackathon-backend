package dao

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"hackathon-backend/model"
)

type ItemDao struct {
	DB *sql.DB
}

const itemColumns = "id, title, description, price, status, created_at"

// FindAll は新しい順に商品を返す。keyword が空でなければ、タイトルか説明文に含むものだけ。
func (d *ItemDao) FindAll(keyword string) ([]model.Item, error) {
	query := "SELECT " + itemColumns + " FROM item"
	args := []any{}
	if keyword != "" {
		// LIKE の特殊文字（% _ \）はエスケープして、文字どおりに探す
		like := "%" + escapeLike(keyword) + "%"
		query += " WHERE title LIKE ? OR description LIKE ?"
		args = append(args, like, like)
	}
	query += " ORDER BY created_at DESC, id DESC"

	rows, err := d.DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("db.Query: %w", err)
	}
	defer rows.Close()

	items := make([]model.Item, 0)
	for rows.Next() {
		var it model.Item
		if err := rows.Scan(&it.Id, &it.Title, &it.Description, &it.Price, &it.Status, &it.CreatedAt); err != nil {
			return nil, fmt.Errorf("rows.Scan: %w", err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows.Err: %w", err)
	}
	return items, nil
}

// FindById は1件返す。無ければ model.ErrNotFound。
func (d *ItemDao) FindById(id string) (model.Item, error) {
	var it model.Item
	err := d.DB.QueryRow("SELECT "+itemColumns+" FROM item WHERE id = ?", id).
		Scan(&it.Id, &it.Title, &it.Description, &it.Price, &it.Status, &it.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Item{}, model.ErrNotFound
	}
	if err != nil {
		return model.Item{}, fmt.Errorf("db.QueryRow: %w", err)
	}
	return it, nil
}

// Insert はトランザクションで1件登録する（失敗したら Rollback）。
func (d *ItemDao) Insert(it model.Item) error {
	tx, err := d.DB.Begin()
	if err != nil {
		return fmt.Errorf("db.Begin: %w", err)
	}
	_, err = tx.Exec(
		"INSERT INTO item (id, title, description, price, status, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		it.Id, it.Title, it.Description, it.Price, it.Status, it.CreatedAt,
	)
	if err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("tx.Exec: %w (rollback: %v)", err, rbErr)
		}
		return fmt.Errorf("tx.Exec: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("tx.Commit: %w", err)
	}
	return nil
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
