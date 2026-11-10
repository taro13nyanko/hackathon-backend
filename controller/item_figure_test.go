package controller

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"hackathon-backend/model"
	"hackathon-backend/usecase"
)

// memFigureStore は dao.ItemDao の SaveFigure・BuyerOf をメモリで真似る。
type memFigureStore struct {
	items     *memItemStore
	purchases *memPurchaseStore
}

func (m *memFigureStore) SaveFigure(id, js, caption string, onlyIfEmpty bool) (bool, error) {
	for i := range m.items.items {
		it := &m.items.items[i]
		if it.Id != id {
			continue
		}
		if onlyIfEmpty && it.Figure != nil {
			return false, nil
		}
		it.Figure, it.FigureCaption = json.RawMessage(js), caption
		return true, nil
	}
	return false, nil
}

func (m *memFigureStore) BuyerOf(itemId string) (string, error) {
	for _, p := range m.purchases.purchases {
		if p.Item.Id == itemId {
			return p.BuyerId, nil
		}
	}
	return "", nil
}

func newFigureSaveHandler() http.Handler {
	members := &usecase.MemberUsecase{Store: &memMemberStore{}}
	a := &Auth{Verifier: fakeVerifier{}, Members: members}
	mux := http.NewServeMux()
	itemStore := &memItemStore{}
	purchases := &memPurchaseStore{items: itemStore}
	items := &usecase.ItemUsecase{Store: itemStore, Figures: &memFigureStore{items: itemStore, purchases: purchases}}
	(&ItemController{Usecase: items, Auth: a}).Register(mux)
	(&PurchaseController{Purchases: &usecase.PurchaseUsecase{Store: purchases}, Items: items, Auth: a}).Register(mux)
	(&MeController{Usecase: members, Auth: a}).Register(mux)
	(&FigureController{Usecase: &usecase.FigureUsecase{}, Items: items, Auth: a}).Register(mux)
	return CORS("", mux)
}

const okFigure = `{"figure":{"name":"本","caption":"よく読まれた本","parts":[{"shape":"box","size":[1,99,0.5],"pos":[0,1,0],"color":"#3355AA"},{"shape":"nope","size":[1],"pos":[0,0,0]}],"face":{"part":0,"style":"smile"},"engine":"main"}}`

func TestSaveFigure(t *testing.T) {
	h := newFigureSaveHandler()
	for _, tok := range []string{"tok-s", "tok-b", "tok-o"} {
		if rec := doAuth(h, "POST", "/me", `{"display_name":"`+tok+`"}`, tok); rec.Code != 201 {
			t.Fatalf("POST /me %s: %d", tok, rec.Code)
		}
	}
	newItem := func() string {
		rec := doAuth(h, "POST", "/items", `{"title":"教科書","price":1200}`, "tok-s")
		var res model.ItemCreateRes
		if rec.Code != 201 || json.Unmarshal(rec.Body.Bytes(), &res) != nil {
			t.Fatalf("POST /items: %d %s", rec.Code, rec.Body)
		}
		return res.Id
	}
	getItem := func(id string) model.Item {
		var it model.Item
		rec := doAuth(h, "GET", "/items/"+id, "", "")
		if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &it) != nil {
			t.Fatalf("GET /items/%s: %d %s", id, rec.Code, rec.Body)
		}
		return it
	}

	a := newItem()
	if rec := doAuth(h, "GET", "/items/"+a, "", ""); !strings.Contains(rec.Body.String(), `"figure":null`) {
		t.Fatalf("模型が無いときは null: %s", rec.Body)
	}
	path := "/items/" + a + "/figure"
	steps := []struct {
		name, token, body string
		want              int
		code              string
	}{
		{"ログインなし", "", okFigure, 401, ""},
		{"関係ない人", "tok-o", okFigure, 403, "forbidden"},
		{"まだ買っていない人", "tok-b", okFigure, 403, "forbidden"},
		{"JSON でない", "tok-s", `{`, 400, "invalid_input"},
		{"figure が無い", "tok-s", `{}`, 400, "invalid_input"},
		{"使える部品が1つも無い", "tok-s", `{"figure":{"name":"x","parts":[{"shape":"nope","size":[1],"pos":[0,0,0]}]}}`, 400, "invalid_input"},
	}
	for _, s := range steps {
		rec := doAuth(h, "PUT", path, s.body, s.token)
		if rec.Code != s.want || (s.code != "" && errCode(rec) != s.code) {
			t.Fatalf("%s: want %d %s, got %d %s", s.name, s.want, s.code, rec.Code, rec.Body)
		}
	}
	if rec := doAuth(h, "PUT", "/items/NOPE/figure", okFigure, "tok-s"); rec.Code != 404 {
		t.Fatalf("無い商品: %d", rec.Code)
	}

	// 出品者が保存 → 直した設計図が返り、商品にも載る
	rec := doAuth(h, "PUT", path, okFigure, "tok-s")
	var res struct{ Figure usecase.Figure }
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &res) != nil {
		t.Fatalf("出品者の保存: %d %s", rec.Code, rec.Body)
	}
	if len(res.Figure.Parts) != 1 || res.Figure.Parts[0].Size[1] != 4 || res.Figure.Parts[0].Color != "#3355aa" || res.Figure.Engine != "" {
		t.Fatalf("範囲内に直して保存する: %+v", res.Figure)
	}
	it := getItem(a)
	var saved usecase.Figure
	if json.Unmarshal(it.Figure, &saved) != nil || len(saved.Parts) != 1 || it.FigureCaption != "よく読まれた本" {
		t.Fatalf("商品に模型が載る: %s / %q", it.Figure, it.FigureCaption)
	}
	// 出品者は作り直して上書きできる
	if rec := doAuth(h, "PUT", path, strings.Replace(okFigure, "よく読まれた本", "作り直し", 1), "tok-s"); rec.Code != 200 || getItem(a).FigureCaption != "作り直し" {
		t.Fatalf("出品者の上書き: %d %s", rec.Code, rec.Body)
	}

	// 購入者は、出品者の模型を上書きできない
	if rec := doAuth(h, "POST", "/items/"+a+"/purchase", "", "tok-b"); rec.Code != 201 {
		t.Fatalf("購入: %d %s", rec.Code, rec.Body)
	}
	if rec := doAuth(h, "PUT", path, okFigure, "tok-b"); rec.Code != 409 || errCode(rec) != "figure_exists" {
		t.Fatalf("購入者の上書き: %d %s", rec.Code, rec.Body)
	}
	if rec := doAuth(h, "PUT", path, okFigure, "tok-o"); rec.Code != 403 {
		t.Fatalf("買っていない人は売り切れ後も 403: %d", rec.Code)
	}

	// 模型の無い商品なら、購入者が保存できる（1回だけ）
	b := newItem()
	doAuth(h, "POST", "/items/"+b+"/purchase", "", "tok-b")
	if rec := doAuth(h, "PUT", "/items/"+b+"/figure", okFigure, "tok-b"); rec.Code != 200 {
		t.Fatalf("購入者の保存: %d %s", rec.Code, rec.Body)
	}
	if rec := doAuth(h, "PUT", "/items/"+b+"/figure", okFigure, "tok-b"); rec.Code != 409 {
		t.Fatalf("購入者の2回目: %d %s", rec.Code, rec.Body)
	}
}
