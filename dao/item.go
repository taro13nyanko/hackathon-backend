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

// itemCols は商品の列（別名 i）。itemCols を SELECT したら、itemScan で受ける。
const itemCols = "i.id, i.title, i.description, i.price, i.status, i.created_at, i.photo_image_id, i.figure_image_id, i.figure_caption"

// itemScan は itemCols を受ける Scan 先と、受けた後に NULL を空文字にする関数を返す。
func itemScan(it *model.Item) ([]any, func()) {
	var photo, figure sql.NullString
	dest := []any{&it.Id, &it.Title, &it.Description, &it.Price, &it.Status, &it.CreatedAt, &photo, &figure, &it.FigureCaption}
	return dest, func() { it.PhotoImageId, it.FigureImageId = photo.String, figure.String }
}

// 出品者の表示名は users から LEFT JOIN で取る（見本データは出品者なし → seller は null）。
const itemSelect = "SELECT " + itemCols + ", u.id, u.display_name" +
	" FROM item i LEFT JOIN users u ON u.id = i.seller_id"

type scanner interface{ Scan(dest ...any) error }

func scanItem(s scanner) (model.Item, error) {
	var it model.Item
	var sellerId, sellerName sql.NullString
	dest, done := itemScan(&it)
	if err := s.Scan(append(dest, &sellerId, &sellerName)...); err != nil {
		return model.Item{}, err
	}
	done()
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
		"INSERT INTO item (id, title, description, price, status, seller_id, photo_image_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		it.Id, it.Title, it.Description, it.Price, it.Status, it.Seller.Id, nullIfEmpty(it.PhotoImageId), it.CreatedAt,
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

// nullIfEmpty は空文字を NULL にする（外部キーの列に ” を入れないため）。
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
