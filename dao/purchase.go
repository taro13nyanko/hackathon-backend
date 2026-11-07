package dao

import (
	"database/sql"
	"errors"
	"fmt"

	"hackathon-backend/model"
	"hackathon-backend/usecase"
)

type PurchaseDao struct {
	DB *sql.DB
}

var _ usecase.PurchaseStore = (*PurchaseDao)(nil)

// Buy は購入を1つのトランザクションで行う。
// 同じ商品を2人が同時に買おうとしても、FOR UPDATE で後の人は先の人の処理が終わるまで待ち、
// 待ったあとに status を読むので sold になっていて ErrAlreadySold になる。
// 念のため purchases.item_id にも UNIQUE を付けている（2重に入れば DB が止める）。
func (d *PurchaseDao) Buy(p model.Purchase) (model.Purchase, error) {
	tx, err := d.DB.Begin()
	if err != nil {
		return model.Purchase{}, fmt.Errorf("db.Begin: %w", err)
	}
	defer tx.Rollback() // Commit の後なら何もしない

	var status string
	var sellerId sql.NullString
	err = tx.QueryRow("SELECT price, status, seller_id FROM item WHERE id = ? FOR UPDATE", p.Item.Id).
		Scan(&p.Price, &status, &sellerId)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Purchase{}, model.ErrNotFound
	}
	if err != nil {
		return model.Purchase{}, fmt.Errorf("select item: %w", err)
	}
	switch {
	case sellerId.Valid && sellerId.String == p.BuyerId:
		return model.Purchase{}, model.ErrOwnItem
	case status != model.ItemOnSale:
		return model.Purchase{}, model.ErrAlreadySold
	}

	if _, err := tx.Exec(
		"INSERT INTO purchases (id, item_id, buyer_id, price, created_at) VALUES (?, ?, ?, ?, ?)",
		p.Id, p.Item.Id, p.BuyerId, p.Price, p.CreatedAt,
	); err != nil {
		if isDuplicate(err) {
			return model.Purchase{}, model.ErrAlreadySold
		}
		return model.Purchase{}, fmt.Errorf("insert purchase: %w", err)
	}
	if _, err := tx.Exec("UPDATE item SET status = ? WHERE id = ?", model.ItemSold, p.Item.Id); err != nil {
		return model.Purchase{}, fmt.Errorf("update item: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return model.Purchase{}, fmt.Errorf("tx.Commit: %w", err)
	}
	return p, nil
}

// FindByBuyer は買った記録を新しい順に返す（商品と出品者の情報つき）。
func (d *PurchaseDao) FindByBuyer(buyerId string) ([]model.Purchase, error) {
	rows, err := d.DB.Query(
		"SELECT p.id, p.price, p.created_at, "+itemCols+", u.id, u.display_name"+
			" FROM purchases p JOIN item i ON i.id = p.item_id LEFT JOIN users u ON u.id = i.seller_id"+
			" WHERE p.buyer_id = ? ORDER BY p.created_at DESC, p.id DESC", buyerId)
	if err != nil {
		return nil, fmt.Errorf("db.Query: %w", err)
	}
	defer rows.Close()
	out := make([]model.Purchase, 0)
	for rows.Next() {
		var p model.Purchase
		var sellerId, sellerName sql.NullString
		dest, done := itemScan(&p.Item)
		if err := rows.Scan(append(append([]any{&p.Id, &p.Price, &p.CreatedAt}, dest...), &sellerId, &sellerName)...); err != nil {
			return nil, fmt.Errorf("rows.Scan: %w", err)
		}
		done()
		if sellerId.Valid {
			p.Item.Seller = &model.Seller{Id: sellerId.String, DisplayName: sellerName.String}
		}
		p.BuyerId = buyerId
		out = append(out, p)
	}
	return out, rows.Err()
}
