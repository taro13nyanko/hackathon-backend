package dao

import (
	"database/sql"
	"fmt"

	"hackathon-backend/model"
)

type UserDao struct {
	DB *sql.DB
}

func (d *UserDao) FindByName(name string) ([]model.User, error) {
	rows, err := d.DB.Query("SELECT id, name, age FROM user WHERE name = ?", name)
	if err != nil {
		return nil, fmt.Errorf("db.Query: %w", err)
	}
	defer rows.Close()

	users := make([]model.User, 0)
	for rows.Next() {
		var u model.User
		if err := rows.Scan(&u.Id, &u.Name, &u.Age); err != nil {
			return nil, fmt.Errorf("rows.Scan: %w", err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows.Err: %w", err)
	}
	return users, nil
}

// FindAll は全ユーザーを返す（0件でも空配列）。
func (d *UserDao) FindAll() ([]model.User, error) {
	rows, err := d.DB.Query("SELECT id, name, age FROM user ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("db.Query: %w", err)
	}
	defer rows.Close()

	users := make([]model.User, 0)
	for rows.Next() {
		var u model.User
		if err := rows.Scan(&u.Id, &u.Name, &u.Age); err != nil {
			return nil, fmt.Errorf("rows.Scan: %w", err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows.Err: %w", err)
	}
	return users, nil
}

// Insert はトランザクションで1件登録する（失敗したら Rollback）。
func (d *UserDao) Insert(u model.User) error {
	tx, err := d.DB.Begin()
	if err != nil {
		return fmt.Errorf("db.Begin: %w", err)
	}
	if _, err := tx.Exec("INSERT INTO user (id, name, age) VALUES (?, ?, ?)", u.Id, u.Name, u.Age); err != nil {
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
