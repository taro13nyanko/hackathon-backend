package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"hackathon-backend/model"
)

// fakeItemStore は商品といいねをメモリで持つ（DAO と同じく、一覧は hidden を除き、liked は viewer で計算する）。
type fakeItemStore struct {
	items   []model.Item
	keyword string          // FindAll に渡されたキーワード
	query   model.ItemQuery // FindAll に渡された条件
	viewer  string          // FindAll・FindById に渡された viewer
	likes   []fakeLike
	err     error
}

type fakeLike struct{ user, item string }

func (f *fakeItemStore) withLikes(it model.Item, viewer string) model.Item {
	it.LikeCount, it.Liked = 0, false
	for _, l := range f.likes {
		if l.item == it.Id {
			it.LikeCount++
			if l.user == viewer {
				it.Liked = true
			}
		}
	}
	return it
}

func (f *fakeItemStore) FindAll(q model.ItemQuery) ([]model.Item, error) {
	f.keyword, f.viewer, f.query = q.Keyword, q.ViewerId, q
	out := []model.Item{}
	for _, it := range f.items {
		if !it.IsHidden() {
			out = append(out, f.withLikes(it, q.ViewerId))
		}
	}
	return out, f.err
}

func (f *fakeItemStore) FindById(id, viewer string) (model.Item, error) {
	f.viewer = viewer
	for _, it := range f.items {
		if it.Id == id {
			return f.withLikes(it, viewer), nil
		}
	}
	return model.Item{}, model.ErrNotFound
}

func (f *fakeItemStore) FindBySeller(sellerId string) ([]model.Item, error) {
	out := []model.Item{}
	for _, it := range f.items {
		if it.Seller != nil && it.Seller.Id == sellerId {
			out = append(out, it)
		}
	}
	return out, nil
}

func (f *fakeItemStore) Insert(it model.Item) error {
	if f.err != nil {
		return f.err
	}
	f.items = append(f.items, it)
	return nil
}

func TestCreateItemValidation(t *testing.T) {
	cases := []struct {
		name string
		req  model.ItemCreateReq
		ok   bool
	}{
		{"正常", model.ItemCreateReq{Title: "教科書", Description: "書き込みなし", Price: 1200}, true},
		{"説明文は空でもよい", model.ItemCreateReq{Title: "教科書", Price: 1200}, true},
		{"最低価格300円ちょうど", model.ItemCreateReq{Title: "a", Price: 300}, true},
		{"最高価格ちょうど", model.ItemCreateReq{Title: "a", Price: 9_999_999}, true},
		{"299円はNG", model.ItemCreateReq{Title: "a", Price: 299}, false},
		{"上限超えはNG", model.ItemCreateReq{Title: "a", Price: 10_000_000}, false},
		{"0円はNG", model.ItemCreateReq{Title: "a", Price: 0}, false},
		{"マイナスはNG", model.ItemCreateReq{Title: "a", Price: -100}, false},
		{"タイトル空はNG", model.ItemCreateReq{Title: "", Price: 1000}, false},
		{"タイトル空白だけはNG", model.ItemCreateReq{Title: "   ", Price: 1000}, false},
		{"タイトル日本語40文字はOK", model.ItemCreateReq{Title: strings.Repeat("あ", 40), Price: 1000}, true},
		{"タイトル41文字はNG", model.ItemCreateReq{Title: strings.Repeat("あ", 41), Price: 1000}, false},
		{"説明1000文字はOK", model.ItemCreateReq{Title: "a", Description: strings.Repeat("い", 1000), Price: 1000}, true},
		{"説明1001文字はNG", model.ItemCreateReq{Title: "a", Description: strings.Repeat("い", 1001), Price: 1000}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			store := &fakeItemStore{}
			uc := &ItemUsecase{Store: store}
			res, err := uc.Create(context.Background(), testSeller, c.req)
			if c.ok {
				if err != nil || len(res.Id) != 26 {
					t.Fatalf("want ok with 26-char id, got id=%q err=%v", res.Id, err)
				}
				if len(store.items) != 1 {
					t.Fatalf("want 1 inserted, got %d", len(store.items))
				}
			} else {
				if !errors.Is(err, ErrInvalidInput) {
					t.Fatalf("want ErrInvalidInput, got %v", err)
				}
				if len(store.items) != 0 {
					t.Fatalf("invalid input must not be saved")
				}
			}
		})
	}
}

func TestCreateItemSetsFields(t *testing.T) {
	store := &fakeItemStore{}
	fixed := time.Date(2026, 11, 1, 12, 0, 0, 500, time.UTC)
	uc := &ItemUsecase{Store: store, Now: func() time.Time { return fixed }}
	if _, err := uc.Create(context.Background(), testSeller, model.ItemCreateReq{Title: "  机  ", Description: " 脚にキズ ", Price: 3000}); err != nil {
		t.Fatal(err)
	}
	got := store.items[0]
	if got.Title != "机" || got.Description != "脚にキズ" {
		t.Errorf("前後の空白を除くこと: title=%q desc=%q", got.Title, got.Description)
	}
	if got.Status != model.ItemOnSale {
		t.Errorf("出品直後は on_sale: got %q", got.Status)
	}
	if !got.CreatedAt.Equal(fixed.Truncate(time.Second)) {
		t.Errorf("created_at: got %v", got.CreatedAt)
	}
}

func TestCreateItemStoreError(t *testing.T) {
	boom := errors.New("db down")
	uc := &ItemUsecase{Store: &fakeItemStore{err: boom}}
	if _, err := uc.Create(context.Background(), testSeller, model.ItemCreateReq{Title: "a", Price: 1000}); !errors.Is(err, boom) {
		t.Fatalf("DB のエラーはそのまま返す: got %v", err)
	}
}

func TestListTrimsKeyword(t *testing.T) {
	store := &fakeItemStore{}
	uc := &ItemUsecase{Store: store}
	if _, err := uc.List(nil, model.ItemQuery{Keyword: "  本  "}); err != nil {
		t.Fatal(err)
	}
	if store.keyword != "本" {
		t.Errorf("keyword: got %q", store.keyword)
	}
}

// 値段の範囲：負は 400、上限より上は上限に収め、min>max は入れ替える（0 は指定なしのまま）。
func TestListPriceRange(t *testing.T) {
	cases := []struct {
		name             string
		min, max         int
		wantMin, wantMax int
		wantErr          error
	}{
		{"指定なし", 0, 0, 0, 0, nil},
		{"以下だけ", 0, 3000, 0, 3000, nil},
		{"以上だけ", 10000, 0, 10000, 0, nil},
		{"逆なら入れ替える", 3000, 1000, 1000, 3000, nil},
		{"上限より上は上限", 0, 50_000_000, 0, ItemPriceMax, nil},
		{"負は 400", -1, 0, 0, 0, ErrInvalidInput},
		{"負は 400（上）", 0, -5, 0, 0, ErrInvalidInput},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			store := &fakeItemStore{}
			uc := &ItemUsecase{Store: store}
			_, err := uc.List(&model.Member{Id: "V"}, model.ItemQuery{Keyword: " 本 ", MinPrice: c.min, MaxPrice: c.max, ViewerId: "偽物"})
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("err: got %v, want %v", err, c.wantErr)
			}
			if c.wantErr != nil {
				return
			}
			if store.query.MinPrice != c.wantMin || store.query.MaxPrice != c.wantMax || store.query.Keyword != "本" || store.query.ViewerId != "V" {
				t.Errorf("DB に渡す条件: %+v", store.query)
			}
		})
	}
}

func TestGetItem(t *testing.T) {
	uc := &ItemUsecase{Store: &fakeItemStore{items: []model.Item{{Id: "X1", Title: "a"}}}}
	if it, err := uc.Get(nil, "X1"); err != nil || it.Title != "a" {
		t.Fatalf("got %+v, %v", it, err)
	}
	if _, err := uc.Get(nil, "nope"); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if _, err := uc.Get(nil, ""); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("want ErrInvalidInput, got %v", err)
	}
}

var testSeller = model.Member{Id: "01JBXSELLER000000000000000", DisplayName: "ota"}

func TestCreateItemNeedsSeller(t *testing.T) {
	store := &fakeItemStore{}
	uc := &ItemUsecase{Store: store}
	if _, err := uc.Create(context.Background(), model.Member{}, model.ItemCreateReq{Title: "a", Price: 1000}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("出品者なしは ErrInvalidInput: %v", err)
	}
	if _, err := uc.Create(context.Background(), testSeller, model.ItemCreateReq{Title: "a", Price: 1000}); err != nil {
		t.Fatal(err)
	}
	if s := store.items[0].Seller; s == nil || s.Id != testSeller.Id || s.DisplayName != "ota" {
		t.Fatalf("出品者を付けること: %+v", s)
	}
}
