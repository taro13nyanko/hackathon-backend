package controller

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"hackathon-backend/model"
	"hackathon-backend/usecase"
)

type memItemStore struct{ items []model.Item }

func (m *memItemStore) FindAll(keyword string) ([]model.Item, error) {
	out := []model.Item{}
	for _, it := range m.items {
		if keyword == "" || strings.Contains(it.Title, keyword) || strings.Contains(it.Description, keyword) {
			out = append(out, it)
		}
	}
	return out, nil
}
func (m *memItemStore) FindById(id string) (model.Item, error) {
	for _, it := range m.items {
		if it.Id == id {
			return it, nil
		}
	}
	return model.Item{}, model.ErrNotFound
}
func (m *memItemStore) Insert(it model.Item) error { m.items = append(m.items, it); return nil }

func newItemHandler() http.Handler {
	c := &ItemController{Usecase: &usecase.ItemUsecase{Store: &memItemStore{}}}
	mux := http.NewServeMux()
	c.Register(mux)
	return CORS("", mux)
}

func TestItemsFlow(t *testing.T) {
	h := newItemHandler()

	// 出品
	rec := do(h, http.MethodPost, "/items", `{"title":"教科書","description":"線形代数","price":1200}`, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /items: want 201, got %d", rec.Code)
	}
	var created model.ItemCreateRes
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || len(created.Id) != 26 {
		t.Fatalf("POST /items body: %s (%v)", rec.Body.String(), err)
	}

	// 一覧
	rec = do(h, http.MethodGet, "/items", "", "")
	var items []model.Item
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &items) != nil || len(items) != 1 {
		t.Fatalf("GET /items: %d %s", rec.Code, rec.Body.String())
	}

	// キーワード検索（日本語は URL エンコードして送る）
	rec = do(h, http.MethodGet, "/items?q=%E7%B7%9A%E5%BD%A2", "", "") // 線形
	if json.Unmarshal(rec.Body.Bytes(), &items) != nil || len(items) != 1 {
		t.Fatalf("GET /items?q=線形: %s", rec.Body.String())
	}
	rec = do(h, http.MethodGet, "/items?q=nothing", "", "")
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("0件は [] を返す（null ではなく）: %s", rec.Body.String())
	}

	// 詳細
	rec = do(h, http.MethodGet, "/items/"+created.Id, "", "")
	var it model.Item
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &it) != nil || it.Title != "教科書" || it.Status != model.ItemOnSale {
		t.Fatalf("GET /items/{id}: %d %s", rec.Code, rec.Body.String())
	}
}

func TestItemsErrors(t *testing.T) {
	h := newItemHandler()
	cases := []struct {
		name, method, target, body string
		want                       int
	}{
		{"安すぎる", http.MethodPost, "/items", `{"title":"a","price":100}`, http.StatusBadRequest},
		{"壊れたJSON", http.MethodPost, "/items", `{"title":`, http.StatusBadRequest},
		{"存在しないID", http.MethodGet, "/items/NOPE", "", http.StatusNotFound},
		{"許可していないメソッド", http.MethodDelete, "/items", "", http.StatusMethodNotAllowed},
		{"プリフライト", http.MethodOptions, "/items", "", http.StatusNoContent},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if rec := do(h, c.method, c.target, c.body, "http://localhost:5173"); rec.Code != c.want {
				t.Fatalf("want %d, got %d", c.want, rec.Code)
			}
		})
	}
}
