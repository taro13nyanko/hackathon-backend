package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// 設計の決まりを、コードで見張るテスト。
//
// 「ID を作るのは model のコンストラクタだけ」（UTTC コード設計ワークショップの模範解答）。
// controller・usecase・dao などで ID を作り始めると、機能ごとに作る場所がばらばらになるので落とす。
func TestIdsAreMadeOnlyInModel(t *testing.T) {
	makesID := regexp.MustCompile(`ulid\.(Make|New|MustNew)\(|uuid\.(New|NewString|NewRandom)\(`)
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (strings.HasPrefix(d.Name(), ".") || d.Name() == "sql") && path != "." {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if filepath.Dir(path) == "model" {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if loc := makesID.FindIndex(b); loc != nil {
			t.Errorf("%s で ID を作っている（model の NewXxx を使うこと）: %q", path, b[loc[0]:loc[1]])
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
