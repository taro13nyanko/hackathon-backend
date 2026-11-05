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

// 出品者の表示名は users から LEFT JOIN で取る（見本データは出品者なし → seller は null）。
const itemSelect = "SELECT i.id, i.title, i.description, i.price, i.status, i.created_at, u.id, u.display_name" +
	" FROM item i LEFT JOIN users u ON u.id = i.seller_id"

type scanner interface{ Scan(dest ...any) error }

func scanItem(s scanner) (model.Item, error) {
	var it model.Item
	var sellerId, sellerName sql.NullString
	if err := s.Scan(&it.Id, &it.Title, &it.Description, &it.Price, &it.Status, &it.CreatedAt, &sellerId, &sellerName); err != nil {
		return model.Item{}, err
	}
	if sellerId.Valid {
		it.Seller = &model.Seller{Id: sellerId.String, DisplayName: sellerName.String}
	}
	return it, nil
}

// FindAll は新しい順に商品を返す。keyword が空でなければ、タイトルか説明文に含むものだけ。
func (d *ItemDao) FindAll(keyword string) ([]model.Item, error) {
	query := itemSelect
	args := []any{}
	if keyword != "" {
		// LIKE の特殊文字（% _ \）はエスケープして、文字どおりに探す
		like := "%" + escapeLike(keyword) + "%"
		query += " WHERE i.title LIKE ? OR i.description LIKE ?"
		args = append(args, like, like)
	}
	query += " ORDER BY i.created_at DESC, i.id DESC"

	rows, err := d.DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("db.Query: %w", err)
	}
	defer rows.Close()

	items := make([]model.Item, 0)
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, fmt.Errorf("rows.Scan: %w", err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows.Err: %w", err)
	}
	return items, nil
}

// FindBySeller は出品者 sellerId の商品を新しい順に返す（マイページ用）。
func (d *ItemDao) FindBySeller(sellerId string) ([]model.Item, error) {
	rows, err := d.DB.Query(itemSelect+" WHERE i.seller_id = ? ORDER BY i.created_at DESC, i.id DESC", sellerId)
	if err != nil {
		return nil, fmt.Errorf("db.Query: %w", err)
	}
	defer rows.Close()
	items := make([]model.Item, 0)
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, fmt.Errorf("rows.Scan: %w", err)
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// FindById は1件返す。無ければ model.ErrNotFound。
func (d *ItemDao) FindById(id string) (model.Item, error) {
	it, err := scanItem(d.DB.QueryRow(itemSelect+" WHERE i.id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return model.Item{}, model.ErrNotFound
	}
	if err != nil {
		return model.Item{}, fmt.Errorf("db.QueryRow: %w", err)
	}
	return it, nil
}

// Insert はトランザクションで1件登録する（失敗したら Rollback）。出品者は必須。
func (d *ItemDao) Insert(it model.Item) error {
	if it.Seller == nil {
		return errors.New("item.Insert: 出品者がない")
	}
	tx, err := d.DB.Begin()
	if err != nil {
		return fmt.Errorf("db.Begin: %w", err)
	}
	_, err = tx.Exec(
		"INSERT INTO item (id, title, description, price, status, seller_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		it.Id, it.Title, it.Description, it.Price, it.Status, it.Seller.Id, it.CreatedAt,
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
