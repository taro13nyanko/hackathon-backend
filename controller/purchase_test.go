package controller

import (
	"encoding/json"
	"strings"
	"testing"

	"hackathon-backend/model"
)

func TestPurchaseFlow(t *testing.T) {
	h := newItemHandler()
	// 出品者 s と買い手 b を登録して、s が出品する
	for _, tok := range []string{"tok-s", "tok-b"} {
		if rec := doAuth(h, "POST", "/me", `{"display_name":"`+tok+`"}`, tok); rec.Code != 201 {
			t.Fatalf("POST /me: %d", rec.Code)
		}
	}
	rec := doAuth(h, "POST", "/items", `{"title":"机","price":3000}`, "tok-s")
	var created model.ItemCreateRes
	json.Unmarshal(rec.Body.Bytes(), &created)

	steps := []struct {
		name, method, path, token string
		want                      int
		code                      string
	}{
		{"ログインなし", "POST", "/items/" + created.Id + "/purchase", "", 401, "unauthorized"},
		{"表示名なし", "POST", "/items/" + created.Id + "/purchase", "tok-x", 403, "profile_required"},
		{"自分の商品", "POST", "/items/" + created.Id + "/purchase", "tok-s", 409, "own_item"},
		{"無い商品", "POST", "/items/NOPE/purchase", "tok-b", 404, "not_found"},
		{"購入", "POST", "/items/" + created.Id + "/purchase", "tok-b", 201, ""},
		{"もう一度", "POST", "/items/" + created.Id + "/purchase", "tok-b", 409, "already_sold"},
	}
	for _, s := range steps {
		rec := doAuth(h, s.method, s.path, "", s.token)
		if rec.Code != s.want || errCode(rec) != s.code {
			t.Fatalf("%s: want %d %q, got %d %s", s.name, s.want, s.code, rec.Code, rec.Body)
		}
	}

	if rec := doAuth(h, "GET", "/items/"+created.Id, "", ""); !strings.Contains(rec.Body.String(), `"status":"sold"`) {
		t.Fatalf("買ったら sold: %s", rec.Body)
	}
	var ps []model.Purchase
	rec = doAuth(h, "GET", "/me/purchases", "", "tok-b")
	if json.Unmarshal(rec.Body.Bytes(), &ps) != nil || len(ps) != 1 || ps[0].Item.Title != "机" || ps[0].Price != 3000 {
		t.Fatalf("GET /me/purchases: %s", rec.Body)
	}
	if rec := doAuth(h, "GET", "/me/purchases", "", "tok-s"); strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("出品者は何も買っていない: %s", rec.Body)
	}
	var mine []model.Item
	rec = doAuth(h, "GET", "/me/items", "", "tok-s")
	if json.Unmarshal(rec.Body.Bytes(), &mine) != nil || len(mine) != 1 || mine[0].Status != model.ItemSold {
		t.Fatalf("GET /me/items: %s", rec.Body)
	}
}
