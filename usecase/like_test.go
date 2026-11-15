package usecase

import (
	"errors"
	"testing"
	"time"

	"hackathon-backend/model"
)

// fakeItemStore に、いいね（LikeStore）と管理者の操作（ModerationStore）を足す。

func (f *fakeItemStore) Like(user, item string, _ time.Time) error {
	for _, l := range f.likes {
		if l.user == user && l.item == item {
			return nil
		}
	}
	f.likes = append(f.likes, fakeLike{user, item})
	return nil
}

func (f *fakeItemStore) Unlike(user, item string) error {
	out := f.likes[:0]
	for _, l := range f.likes {
		if l.user != user || l.item != item {
			out = append(out, l)
		}
	}
	f.likes = out
	return nil
}

func (f *fakeItemStore) CountLikes(item string) (int, error) {
	n := 0
	for _, l := range f.likes {
		if l.item == item {
			n++
		}
	}
	return n, nil
}

// FindLikedBy は後からいいねした順（新しい順）に返す。
func (f *fakeItemStore) FindLikedBy(user string) ([]model.Item, error) {
	out := []model.Item{}
	for i := len(f.likes) - 1; i >= 0; i-- {
		if f.likes[i].user != user {
			continue
		}
		it, _ := f.FindById(f.likes[i].item, user)
		if !it.IsHidden() {
			out = append(out, it)
		}
	}
	return out, nil
}

func (f *fakeItemStore) FindByModeration(status, viewer string) ([]model.Item, error) {
	out := []model.Item{}
	for _, it := range f.items {
		if it.ModerationStatus == status {
			out = append(out, f.withLikes(it, viewer))
		}
	}
	return out, nil
}

func (f *fakeItemStore) SetModeration(id, status string, reason *string) error {
	for i := range f.items {
		if f.items[i].Id == id {
			f.items[i].ModerationStatus = status
			if reason != nil {
				f.items[i].ModerationReason = *reason
			}
			return nil
		}
	}
	return model.ErrNotFound
}

var (
	likeSeller = model.Member{Id: "S", Role: model.RoleMember}
	likeBuyer  = model.Member{Id: "B", Role: model.RoleMember}
	likeAdmin  = model.Member{Id: "A", Role: model.RoleAdmin}
)

func moderatedItems() *fakeItemStore {
	s := &model.Seller{Id: "S", DisplayName: "ota"}
	return &fakeItemStore{items: []model.Item{
		{Id: "OK", Seller: s, ModerationStatus: model.ModerationOK},
		{Id: "REVIEW", Seller: s, ModerationStatus: model.ModerationReview, ModerationReason: "医薬品かも"},
		{Id: "HIDDEN", Seller: s, ModerationStatus: model.ModerationHidden, ModerationReason: "模造銃"},
	}}
}

func TestLikeRules(t *testing.T) {
	cases := []struct {
		name      string
		me        model.Member
		item      string
		like      bool
		wantErr   error
		wantCount int
	}{
		{"いいねする", likeBuyer, "OK", true, nil, 1},
		{"2回目も成功（数は増えない）", likeBuyer, "OK", true, nil, 1},
		{"別の人", likeAdmin, "OK", true, nil, 2},
		{"自分の商品にもできる", likeSeller, "OK", true, nil, 3},
		{"要確認の商品にもできる", likeBuyer, "REVIEW", true, nil, 1},
		{"取り消す", likeBuyer, "OK", false, nil, 2},
		{"付いていない取り消しも成功", likeBuyer, "OK", false, nil, 2},
		{"非公開の商品は 404", likeSeller, "HIDDEN", true, model.ErrNotFound, 0},
		{"非公開の商品の取り消しも 404", likeAdmin, "HIDDEN", false, model.ErrNotFound, 0},
		{"無い商品は 404", likeBuyer, "NOPE", true, model.ErrNotFound, 0},
		{"ID が空", likeBuyer, "", true, ErrInvalidInput, 0},
		{"会員でない", model.Member{}, "OK", true, ErrInvalidInput, 0},
	}
	uc := &LikeUsecase{Store: nil, Items: moderatedItems()}
	uc.Store = uc.Items.(*fakeItemStore)
	for _, c := range cases {
		var res model.LikeRes
		var err error
		if c.like {
			res, err = uc.Like(c.me, c.item)
		} else {
			res, err = uc.Unlike(c.me, c.item)
		}
		if !errors.Is(err, c.wantErr) {
			t.Fatalf("%s: err want %v, got %v", c.name, c.wantErr, err)
		}
		if c.wantErr == nil && (res.Liked != c.like || res.LikeCount != c.wantCount) {
			t.Fatalf("%s: want liked=%v count=%d, got %+v", c.name, c.like, c.wantCount, res)
		}
	}
}

func TestLikeRateLimitAndList(t *testing.T) {
	store := moderatedItems()
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	lim := NewLimiter(LikePerMinute, time.Minute)
	lim.Now = func() time.Time { return now }
	uc := &LikeUsecase{Store: store, Items: store, Limit: lim}
	for i := 0; i < LikePerMinute; i++ {
		if _, err := uc.Like(likeBuyer, "OK"); err != nil {
			t.Fatal(i, err)
		}
	}
	if _, err := uc.Unlike(likeBuyer, "OK"); !errors.Is(err, ErrTooMany) {
		t.Fatalf("いいねと取り消しを合わせて1分%d回まで: %v", LikePerMinute, err)
	}
	now = now.Add(61 * time.Second)
	if _, err := uc.Like(likeBuyer, "REVIEW"); err != nil {
		t.Fatalf("1分たてば使える: %v", err)
	}

	// 一覧：新しい順。非公開の商品は出さない。ほかの人の商品の理由は消す
	store.likes = append(store.likes, fakeLike{"B", "HIDDEN"})
	items, err := uc.List(likeBuyer)
	if err != nil || len(items) != 2 || items[0].Id != "REVIEW" || items[1].Id != "OK" {
		t.Fatalf("いいねの一覧: %+v %v", items, err)
	}
	if items[0].ModerationReason != "" || !items[0].Liked || items[0].ModerationStatus != model.ModerationReview {
		t.Fatalf("状態は見せ、理由は出品者と管理者だけ: %+v", items[0])
	}
	store.likes = append(store.likes, fakeLike{"S", "REVIEW"})
	if items, _ := uc.List(likeSeller); len(items) != 1 || items[0].ModerationReason != "医薬品かも" {
		t.Fatalf("出品者は自分の商品の理由が見える: %+v", items)
	}
}
