package dao

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/go-sql-driver/mysql"

	"hackathon-backend/model"
)

type MemberDao struct {
	DB *sql.DB
}

// FindByFirebaseUID は1件返す。無ければ model.ErrNotFound。
func (d *MemberDao) FindByFirebaseUID(uid string) (model.Member, error) {
	var m model.Member
	err := d.DB.QueryRow(
		"SELECT id, firebase_uid, display_name, created_at FROM users WHERE firebase_uid = ?", uid,
	).Scan(&m.Id, &m.FirebaseUID, &m.DisplayName, &m.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Member{}, model.ErrNotFound
	}
	if err != nil {
		return model.Member{}, fmt.Errorf("db.QueryRow: %w", err)
	}
	return m, nil
}

// isDuplicate は UNIQUE 違反（MySQL のエラー 1062 Duplicate entry）か。
func isDuplicate(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == 1062
}

// Insert は1件登録する。firebase_uid が重複したら model.ErrAlreadyRegistered。
func (d *MemberDao) Insert(m model.Member) error {
	_, err := d.DB.Exec(
		"INSERT INTO users (id, firebase_uid, display_name, created_at) VALUES (?, ?, ?, ?)",
		m.Id, m.FirebaseUID, m.DisplayName, m.CreatedAt,
	)
	if isDuplicate(err) {
		return model.ErrAlreadyRegistered
	}
	if err != nil {
		return fmt.Errorf("db.Exec: %w", err)
	}
	return nil
}
