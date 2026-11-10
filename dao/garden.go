package dao

import (
	"database/sql"
	"fmt"
	"time"

	"hackathon-backend/model"
	"hackathon-backend/usecase"
)

// GardenDao は箱庭の配置（garden_spots テーブル、sql/007_garden.sql）。
type GardenDao struct {
	DB *sql.DB
}

var _ usecase.GardenStore = (*GardenDao)(nil)

func (d *GardenDao) Spots(userId string) ([]model.GardenSpot, error) {
	rows, err := d.DB.Query("SELECT item_id, x, z FROM garden_spots WHERE user_id = ? ORDER BY item_id", userId)
	if err != nil {
		return nil, fmt.Errorf("db.Query: %w", err)
	}
	defer rows.Close()
	out := make([]model.GardenSpot, 0)
	for rows.Next() {
		var s model.GardenSpot
		if err := rows.Scan(&s.ItemId, &s.X, &s.Z); err != nil {
			return nil, fmt.Errorf("rows.Scan: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (d *GardenDao) GardenItemIds(userId string) (map[string]bool, error) {
	rows, err := d.DB.Query("SELECT id FROM item WHERE seller_id = ? UNION SELECT item_id FROM purchases WHERE buyer_id = ?", userId, userId)
	if err != nil {
		return nil, fmt.Errorf("db.Query: %w", err)
	}
	defer rows.Close()
	ids := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("rows.Scan: %w", err)
		}
		ids[id] = true
	}
	return ids, rows.Err()
}

func (d *GardenDao) ReplaceSpots(userId string, spots []model.GardenSpot, at time.Time) (err error) {
	tx, err := d.DB.Begin()
	if err != nil {
		return fmt.Errorf("db.Begin: %w", err)
	}
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				err = fmt.Errorf("%w (rollback: %v)", err, rbErr)
			}
		}
	}()
	if _, err = tx.Exec("DELETE FROM garden_spots WHERE user_id = ?", userId); err != nil {
		return fmt.Errorf("delete spots: %w", err)
	}
	for _, s := range spots {
		if _, err = tx.Exec("INSERT INTO garden_spots (user_id, item_id, x, z, updated_at) VALUES (?, ?, ?, ?, ?)",
			userId, s.ItemId, s.X, s.Z, at); err != nil {
			return fmt.Errorf("insert spot: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("tx.Commit: %w", err)
	}
	return nil
}
