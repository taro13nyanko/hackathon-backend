package dao

import (
	"strings"
	"testing"
)

func TestDSN(t *testing.T) {
	t.Setenv("MYSQL_USER", "uttc")
	t.Setenv("MYSQL_PWD", "pw")
	t.Setenv("MYSQL_DATABASE", "hackathon")
	t.Setenv("MYSQL_HOST", "unix(/cloudsql/proj:us-central1:uttc)")
	dsn, err := DSN()
	if err != nil || !strings.HasPrefix(dsn, "uttc:pw@unix(/cloudsql/proj:us-central1:uttc)/hackathon?") {
		t.Fatalf("got %q %v", dsn, err)
	}

	t.Setenv("MYSQL_HOST", "")
	if dsn, _ := DSN(); !strings.Contains(dsn, "@tcp(127.0.0.1:3306)/") {
		t.Fatalf("default host: %q", dsn)
	}

	t.Setenv("MYSQL_PWD", "")
	if _, err := DSN(); err == nil || !strings.Contains(err.Error(), "MYSQL_PWD") {
		t.Fatalf("missing pwd should error, got %v", err)
	}
}
