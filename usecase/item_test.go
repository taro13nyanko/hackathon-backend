package usecase

import (
	"errors"
	"strings"
	"testing"
	"time"

	"hackathon-backend/model"
)

type fakeItemStore struct {
	items   []model.Item
	keyword string // FindAll に渡されたキーワード
	err     error
}

func (f *fakeItemStore) FindAll(keyword string) ([]model.Item, error) {
	f.keyword = keyword
	return append([]model.Item{}, f.items...), f.err
}

func (f *fakeItemStore) FindById(id string) (model.Item, error) {
	for _, it := range f.items {
		if it.Id == id {
			return it, nil
		}
	}
	return model.Item{}, model.ErrNotFound
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
			res, err := uc.Create(c.req)
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
	if _, err := uc.Create(model.ItemCreateReq{Title: "  机  ", Description: " 脚にキズ ", Price: 3000}); err != nil {
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
	if _, err := uc.Create(model.ItemCreateReq{Title: "a", Price: 1000}); !errors.Is(err, boom) {
		t.Fatalf("DB のエラーはそのまま返す: got %v", err)
	}
}

func TestListTrimsKeyword(t *testing.T) {
	store := &fakeItemStore{}
	uc := &ItemUsecase{Store: store}
	if _, err := uc.List("  本  "); err != nil {
		t.Fatal(err)
	}
	if store.keyword != "本" {
		t.Errorf("keyword: got %q", store.keyword)
	}
}

func TestGetItem(t *testing.T) {
	uc := &ItemUsecase{Store: &fakeItemStore{items: []model.Item{{Id: "X1", Title: "a"}}}}
	if it, err := uc.Get("X1"); err != nil || it.Title != "a" {
		t.Fatalf("got %+v, %v", it, err)
	}
	if _, err := uc.Get("nope"); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if _, err := uc.Get(""); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("want ErrInvalidInput, got %v", err)
	}
}
