package dao

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// DSN は環境変数から接続文字列を組み立てる。
//
//	Cloud Run : MYSQL_HOST=unix(/cloudsql/<プロジェクトID>:us-central1:uttc)
//	ローカル   : MYSQL_HOST=tcp(127.0.0.1:3306)
//
// MYSQL_HOST を省略すると tcp(127.0.0.1:3306) を使う。
func DSN() (string, error) {
	user := os.Getenv("MYSQL_USER")
	pwd := os.Getenv("MYSQL_PWD")
	host := os.Getenv("MYSQL_HOST")
	database := os.Getenv("MYSQL_DATABASE")

	var missing []string
	if user == "" {
		missing = append(missing, "MYSQL_USER")
	}
	if pwd == "" {
		missing = append(missing, "MYSQL_PWD")
	}
	if database == "" {
		missing = append(missing, "MYSQL_DATABASE")
	}
	if len(missing) > 0 {
		return "", fmt.Errorf("環境変数が未設定です: %s", strings.Join(missing, ", "))
	}
	if host == "" {
		host = "tcp(127.0.0.1:3306)"
	}
	// parseTime=true で DATETIME 列を time.Time として読める。
	return fmt.Sprintf("%s:%s@%s/%s?parseTime=true&loc=Local", user, pwd, host, database), nil
}

// Open は DB に接続し、Ping で疎通を確認する。
func Open() (*sql.DB, error) {
	dsn, err := DSN()
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	// 共有コア(0.614GB)の Cloud SQL は同時接続数が少ないので控えめに。
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("db.Ping: %w", err)
	}
	return db, nil
}
