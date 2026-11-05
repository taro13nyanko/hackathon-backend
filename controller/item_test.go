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
func (m *memItemStore) FindBySeller(sellerId string) ([]model.Item, error) {
	out := []model.Item{}
	for _, it := range m.items {
		if it.Seller != nil && it.Seller.Id == sellerId {
			out = append(out, it)
		}
	}
	return out, nil
}
func (m *memItemStore) Insert(it model.Item) error { m.items = append(m.items, it); return nil }

// memPurchaseStore は DAO と同じ決まり（無い→404、自分の→own_item、売り切れ→already_sold）をメモリで真似る。
type memPurchaseStore struct {
	items     *memItemStore
	purchases []model.Purchase
}

func (m *memPurchaseStore) Buy(p model.Purchase) (model.Purchase, error) {
	for i := range m.items.items {
		it := &m.items.items[i]
		if it.Id != p.Item.Id {
			continue
		}
		if it.Seller != nil && it.Seller.Id == p.BuyerId {
			return model.Purchase{}, model.ErrOwnItem
		}
		if it.Status != model.ItemOnSale {
			return model.Purchase{}, model.ErrAlreadySold
		}
		it.Status = model.ItemSold
		p.Price, p.Item = it.Price, *it
		m.purchases = append(m.purchases, p)
		return p, nil
	}
	return model.Purchase{}, model.ErrNotFound
}
func (m *memPurchaseStore) FindByBuyer(buyerId string) ([]model.Purchase, error) {
	out := []model.Purchase{}
	for _, p := range m.purchases {
		if p.BuyerId == buyerId {
			out = append(out, p)
		}
	}
	return out, nil
}

// newItemHandler は /items と /me をつないだテスト用サーバー。
// 偽のトークン確認係は "tok-<uid>" を uid として受け付ける。
func newItemHandler() http.Handler {
	members := &usecase.MemberUsecase{Store: &memMemberStore{}}
	a := &Auth{Verifier: fakeVerifier{}, Members: members}
	mux := http.NewServeMux()
	itemStore := &memItemStore{}
	items := &usecase.ItemUsecase{Store: itemStore}
	(&ItemController{Usecase: items, Auth: a}).Register(mux)
	(&PurchaseController{Purchases: &usecase.PurchaseUsecase{Store: &memPurchaseStore{items: itemStore}}, Items: items, Auth: a}).Register(mux)
	(&MeController{Usecase: members, Auth: a}).Register(mux)
	return CORS("", mux)
}

func TestItemsFlow(t *testing.T) {
	h := newItemHandler()

	// 出品（ログイン＋プロフィール登録済み）
	if rec := doAuth(h, http.MethodPost, "/me", `{"display_name":"ota"}`, "tok-uid-1"); rec.Code != http.StatusCreated {
		t.Fatalf("POST /me: %d %s", rec.Code, rec.Body)
	}
	rec := doAuth(h, http.MethodPost, "/items", `{"title":"教科書","description":"線形代数","price":1200}`, "tok-uid-1")
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
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &it) != nil || it.Title != "教科書" || it.Status != model.ItemOnSale ||
		it.Seller == nil || it.Seller.DisplayName != "ota" {
		t.Fatalf("GET /items/{id}: %d %s", rec.Code, rec.Body.String())
	}
}

func TestItemsErrors(t *testing.T) {
	h := newItemHandler()
	cases := []struct {
		name, method, target, body string
		want                       int
	}{
		{"ログインなしで出品", http.MethodPost, "/items", `{"title":"a","price":1000}`, http.StatusUnauthorized},
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
