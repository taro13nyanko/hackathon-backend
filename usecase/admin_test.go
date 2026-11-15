package usecase

import (
	"errors"
	"strings"
	"testing"

	"hackathon-backend/model"
)

func TestItemVisibility(t *testing.T) {
	cases := []struct {
		name       string
		viewer     *model.Member
		id         string
		wantErr    error
		wantReason string
	}{
		{"未ログイン・公開中", nil, "OK", nil, ""},
		{"未ログイン・要確認は状態だけ（理由は空）", nil, "REVIEW", nil, ""},
		{"ほかの人・要確認は理由なし", &likeBuyer, "REVIEW", nil, ""},
		{"出品者・要確認は理由つき", &likeSeller, "REVIEW", nil, "医薬品かも"},
		{"管理者・要確認は理由つき", &likeAdmin, "REVIEW", nil, "医薬品かも"},
		{"未ログイン・非公開は 404", nil, "HIDDEN", model.ErrNotFound, ""},
		{"ほかの人・非公開は 404", &likeBuyer, "HIDDEN", model.ErrNotFound, ""},
		{"出品者・非公開は見える", &likeSeller, "HIDDEN", nil, "模造銃"},
		{"管理者・非公開は見える", &likeAdmin, "HIDDEN", nil, "模造銃"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			uc := &ItemUsecase{Store: moderatedItems()}
			it, err := uc.Get(c.viewer, c.id)
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("err: want %v, got %v", c.wantErr, err)
			}
			if err == nil && (it.ModerationReason != c.wantReason || it.ModerationStatus == "") {
				t.Fatalf("理由: want %q, got %+v", c.wantReason, it)
			}
		})
	}
}

func TestListHidesHiddenAndReasons(t *testing.T) {
	store := moderatedItems()
	store.likes = []fakeLike{{"B", "OK"}, {"S", "OK"}}
	uc := &ItemUsecase{Store: store}
	for _, c := range []struct {
		name       string
		viewer     *model.Member
		wantReason string
		wantLiked  bool
	}{
		{"未ログイン", nil, "", false},
		{"ほかの人", &likeBuyer, "", true},
		{"出品者", &likeSeller, "医薬品かも", true},
		{"管理者", &likeAdmin, "医薬品かも", false},
	} {
		items, err := uc.List(c.viewer, model.ItemQuery{})
		if err != nil || len(items) != 2 {
			t.Fatalf("%s: 非公開は一覧に出さない: %+v %v", c.name, items, err)
		}
		if store.viewer != viewerId(c.viewer) {
			t.Fatalf("%s: liked は本人として数える: %q", c.name, store.viewer)
		}
		if items[0].LikeCount != 2 || items[0].Liked != c.wantLiked || items[1].ModerationReason != c.wantReason {
			t.Fatalf("%s: %+v", c.name, items)
		}
	}
	// マイページ（自分の出品）は非公開も理由つきで見える
	if items, _ := uc.Mine(likeSeller); len(items) != 3 || items[2].ModerationReason != "模造銃" {
		t.Fatalf("自分の出品: %+v", items)
	}
}

func TestHiddenItemsCannotBeUsed(t *testing.T) {
	store := moderatedItems()
	// やりとりは始められない
	dm := &DMUsecase{Store: &fakeDMStore{}, Items: store}
	if _, _, err := dm.Start(likeBuyer, "HIDDEN"); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("非公開の商品とのやりとり: %v", err)
	}
	if _, _, err := dm.Start(likeBuyer, "REVIEW"); err != nil {
		t.Fatalf("要確認の商品とはやりとりできる: %v", err)
	}
	// 模型は保存できない（出品者でも）
	uc := &ItemUsecase{Store: store, Figures: &fakeFigureStore{saved: true}}
	fig := &Figure{Name: "本", Parts: []FigurePart{{Shape: "box", Size: []float64{1, 1, 1}, Pos: []float64{0, 0.5, 0}}}}
	if _, err := uc.SaveFigure(likeSeller, "HIDDEN", fig); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("非公開の商品への模型の保存: %v", err)
	}
}

func TestAdminModeration(t *testing.T) {
	cases := []struct {
		name       string
		me         model.Member
		id         string
		req        model.ModerationUpdateReq
		wantErr    error
		wantStatus string
		wantReason string
	}{
		{"会員は 403", likeSeller, "REVIEW", model.ModerationUpdateReq{Status: "hidden"}, model.ErrForbidden, "", ""},
		{"知らない状態", likeAdmin, "REVIEW", model.ModerationUpdateReq{Status: "review"}, ErrInvalidInput, "", ""},
		{"状態が空", likeAdmin, "REVIEW", model.ModerationUpdateReq{}, ErrInvalidInput, "", ""},
		{"メモが長すぎる", likeAdmin, "REVIEW", model.ModerationUpdateReq{Status: "ok", Note: strings.Repeat("あ", ModerationNoteMax+1)}, ErrInvalidInput, "", ""},
		{"無い商品", likeAdmin, "NOPE", model.ModerationUpdateReq{Status: "ok"}, model.ErrNotFound, "", ""},
		{"メモなしで非公開（理由はそのまま）", likeAdmin, "REVIEW", model.ModerationUpdateReq{Status: "hidden"}, nil, "hidden", "医薬品かも"},
		{"メモ120字で公開（理由を上書き）", likeAdmin, "HIDDEN", model.ModerationUpdateReq{Status: "ok", Note: " " + strings.Repeat("あ", ModerationNoteMax) + " "}, nil, "ok", strings.Repeat("あ", ModerationNoteMax)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			store := moderatedItems()
			uc := &AdminUsecase{Store: store, Items: store}
			it, err := uc.SetModeration(c.me, c.id, c.req)
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("err: want %v, got %v", c.wantErr, err)
			}
			if err == nil && (it.ModerationStatus != c.wantStatus || it.ModerationReason != c.wantReason || it.Id != c.id) {
				t.Fatalf("更新後の商品を返す: %+v", it)
			}
		})
	}
}

func TestAdminQueue(t *testing.T) {
	store := moderatedItems()
	uc := &AdminUsecase{Store: store, Items: store}
	cases := []struct {
		name    string
		me      model.Member
		status  string
		wantErr error
		wantId  string
	}{
		{"既定は要確認", likeAdmin, "", nil, "REVIEW"},
		{"非公開", likeAdmin, "hidden", nil, "HIDDEN"},
		{"ok は選べない", likeAdmin, "ok", ErrInvalidInput, ""},
		{"会員は 403", likeBuyer, "", model.ErrForbidden, ""},
	}
	for _, c := range cases {
		items, err := uc.Queue(c.me, c.status)
		if !errors.Is(err, c.wantErr) {
			t.Fatalf("%s: err want %v, got %v", c.name, c.wantErr, err)
		}
		if err == nil && (len(items) != 1 || items[0].Id != c.wantId || items[0].ModerationReason == "") {
			t.Fatalf("%s: 理由つきで返す: %+v", c.name, items)
		}
	}
}

func TestRegisterMemberRole(t *testing.T) {
	uc := &MemberUsecase{Store: &fakeMemberStore{}}
	m, err := uc.Register("uid-r", model.MemberCreateReq{DisplayName: "ota"})
	if err != nil || m.Role != model.RoleMember || m.IsAdmin() {
		t.Fatalf("登録した人は member（管理者は DB で決める）: %+v %v", m, err)
	}
}
