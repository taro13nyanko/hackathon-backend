package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"hackathon-backend/model"
	"hackathon-backend/usecase"
)

// ---- テスト用の偽物（いいね・管理者・やりとり・出品の確認） ----

func (m *memItemStore) Like(user, item string, _ time.Time) error {
	for _, l := range m.likes {
		if l == [2]string{user, item} {
			return nil
		}
	}
	m.likes = append(m.likes, [2]string{user, item})
	return nil
}
func (m *memItemStore) Unlike(user, item string) error {
	out := m.likes[:0]
	for _, l := range m.likes {
		if l != [2]string{user, item} {
			out = append(out, l)
		}
	}
	m.likes = out
	return nil
}
func (m *memItemStore) CountLikes(item string) (int, error) {
	n := 0
	for _, l := range m.likes {
		if l[1] == item {
			n++
		}
	}
	return n, nil
}
func (m *memItemStore) FindLikedBy(user string) ([]model.Item, error) {
	out := []model.Item{}
	for i := len(m.likes) - 1; i >= 0; i-- {
		if m.likes[i][0] != user {
			continue
		}
		if it, err := m.FindById(m.likes[i][1], user); err == nil && !it.IsHidden() {
			out = append(out, it)
		}
	}
	return out, nil
}
func (m *memItemStore) FindByModeration(status, viewer string) ([]model.Item, error) {
	out := []model.Item{}
	for _, it := range m.items {
		if it.ModerationStatus == status {
			out = append(out, m.withLikes(it, viewer))
		}
	}
	return out, nil
}
func (m *memItemStore) SetModeration(id, status string, reason *string) error {
	for i := range m.items {
		if m.items[i].Id == id {
			m.items[i].ModerationStatus = status
			if reason != nil {
				m.items[i].ModerationReason = *reason
			}
			return nil
		}
	}
	return model.ErrNotFound
}

// memDMStore はやりとりを始めるところだけ真似る（商品の確かめは usecase がする）。
type memDMStore struct{}

func (memDMStore) FindOrCreate(c model.Conversation) (string, bool, error) { return "C1", true, nil }
func (memDMStore) Get(id, viewer string) (model.Conversation, error) {
	return model.Conversation{Id: id}, nil
}
func (memDMStore) ListFor(string) ([]model.Conversation, error)     { return nil, nil }
func (memDMStore) Participants(string) (string, string, error)      { return "", "", model.ErrNotFound }
func (memDMStore) Messages(string, string) ([]model.Message, error) { return nil, nil }
func (memDMStore) InsertMessage(string, model.Message) error        { return nil }
func (memDMStore) UnreadCounts(string) (map[string]int, error)      { return map[string]int{}, nil }
func (memDMStore) MarkRead(string, string, string, time.Time) (string, error) {
	return "", nil
}

// fakeModerator は商品名で判定を変える偽の確認係。
type fakeModerator struct{}

func (fakeModerator) Moderate(_ context.Context, in usecase.ModerationInput) (usecase.ModerationResult, error) {
	switch {
	case strings.Contains(in.Title, "模造銃"):
		return usecase.ModerationResult{Verdict: "block", Reason: "模造銃は出品できません", Categories: []string{"weapon"}}, nil
	case strings.Contains(in.Title, "薬"):
		return usecase.ModerationResult{Verdict: "review", Reason: "医薬品かもしれません", Categories: []string{"drug"}}, nil
	case strings.Contains(in.Title, "故障"):
		return usecase.ModerationResult{}, context.DeadlineExceeded
	}
	return usecase.ModerationResult{Verdict: "ok", Categories: []string{}}, nil
}

// setupUsers は出品者 s・ほかの人 o・管理者 a を登録する（管理者は DB で role を変えたのと同じにする）。
func setupUsers(t *testing.T, h http.Handler, members *memMemberStore) {
	t.Helper()
	for _, tok := range []string{"tok-s", "tok-o", "tok-a"} {
		if rec := doAuth(h, "POST", "/me", `{"display_name":"`+tok+`"}`, tok); rec.Code != 201 {
			t.Fatalf("POST /me %s: %d", tok, rec.Code)
		}
	}
	for i := range members.members {
		if members.members[i].FirebaseUID == "a" {
			members.members[i].Role = model.RoleAdmin
		}
	}
}

func createItem(t *testing.T, h http.Handler, title string) model.ItemCreateRes {
	t.Helper()
	rec := doAuth(h, "POST", "/items", `{"title":"`+title+`","price":1200}`, "tok-s")
	var res model.ItemCreateRes
	if rec.Code != 201 || json.Unmarshal(rec.Body.Bytes(), &res) != nil {
		t.Fatalf("POST /items %s: %d %s", title, rec.Code, rec.Body)
	}
	return res
}

// ---- 出品の確認 ----

func TestCreateItemModerationHTTP(t *testing.T) {
	h, store, members := newAppHandler(fakeModerator{}, nil)
	setupUsers(t, h, members)

	// block → 422 prohibited、理由とカテゴリを返し、保存しない
	rec := doAuth(h, "POST", "/items", `{"title":"模造銃","price":1200}`, "tok-s")
	var body struct {
		Error      string   `json:"error"`
		Reason     string   `json:"reason"`
		Categories []string `json:"categories"`
	}
	if rec.Code != http.StatusUnprocessableEntity || json.Unmarshal(rec.Body.Bytes(), &body) != nil ||
		body.Error != "prohibited" || body.Reason != "模造銃は出品できません" || len(body.Categories) != 1 || body.Categories[0] != "weapon" {
		t.Fatalf("block: %d %s", rec.Code, rec.Body)
	}
	if len(store.items) != 0 {
		t.Fatalf("block は保存しない")
	}

	cases := []struct {
		title, status, reason string
	}{
		{"教科書", "ok", ""},
		{"風邪薬", "review", "医薬品かもしれません"},
		{"故障中", "review", usecase.ModerationFailedReason},
	}
	for _, c := range cases {
		res := createItem(t, h, c.title)
		if len(res.Id) != 26 || res.ModerationStatus != c.status || res.ModerationReason != c.reason {
			t.Fatalf("%s: %+v", c.title, res)
		}
	}
	// レスポンスの形（キー名）を固定する
	rec = doAuth(h, "POST", "/items", `{"title":"ノート","price":300}`, "tok-s")
	for _, key := range []string{`"id":`, `"moderation_status":"ok"`, `"moderation_reason":""`} {
		if !strings.Contains(rec.Body.String(), key) {
			t.Fatalf("POST /items のレスポンスに %s: %s", key, rec.Body)
		}
	}
}

// ---- ロールと非公開 ----

func TestMeHasRole(t *testing.T) {
	h, _, members := newAppHandler(nil, nil)
	rec := doAuth(h, "POST", "/me", `{"display_name":"ota"}`, "tok-u")
	if rec.Code != 201 || !strings.Contains(rec.Body.String(), `"role":"member"`) {
		t.Fatalf("POST /me は role を返す: %d %s", rec.Code, rec.Body)
	}
	members.members[0].Role = model.RoleAdmin
	if rec := doAuth(h, "GET", "/me", "", "tok-u"); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"role":"admin"`) {
		t.Fatalf("GET /me は role を返す: %d %s", rec.Code, rec.Body)
	}
}

func TestHiddenAndAdminHTTP(t *testing.T) {
	h, _, members := newAppHandler(fakeModerator{}, nil)
	setupUsers(t, h, members)
	review := createItem(t, h, "風邪薬").Id
	target := createItem(t, h, "教科書").Id

	steps := []struct {
		name, method, path, body, token string
		want                            int
		code                            string
	}{
		{"管理画面・ログインなし", "GET", "/admin/items", "", "", 401, "unauthorized"},
		{"管理画面・未登録", "GET", "/admin/items", "", "tok-new", 403, "profile_required"},
		{"管理画面・会員", "GET", "/admin/items", "", "tok-o", 403, "forbidden"},
		{"管理画面・出品者", "PUT", "/admin/items/" + target + "/moderation", `{"status":"hidden"}`, "tok-s", 403, "forbidden"},
		{"管理画面・知らない一覧", "GET", "/admin/items?status=ok", "", "tok-a", 400, "invalid_input"},
		{"状態が review", "PUT", "/admin/items/" + target + "/moderation", `{"status":"review"}`, "tok-a", 400, "invalid_input"},
		{"メモが長い", "PUT", "/admin/items/" + target + "/moderation", `{"status":"hidden","note":"` + strings.Repeat("あ", 121) + `"}`, "tok-a", 400, "invalid_input"},
		{"壊れたJSON", "PUT", "/admin/items/" + target + "/moderation", `{`, "tok-a", 400, "invalid_input"},
		{"無い商品", "PUT", "/admin/items/NOPE/moderation", `{"status":"hidden"}`, "tok-a", 404, "not_found"},
		{"非公開にする", "PUT", "/admin/items/" + target + "/moderation", `{"status":"hidden","note":"写真が不適切"}`, "tok-a", 200, ""},
		// 非公開の商品：出品者と管理者だけ見られる。ほかは 404。使えない
		{"詳細・未ログイン", "GET", "/items/" + target, "", "", 404, "not_found"},
		{"詳細・ほかの人", "GET", "/items/" + target, "", "tok-o", 404, "not_found"},
		{"詳細・無効なトークンは未ログイン扱い", "GET", "/items/" + target, "", "garbage", 404, "not_found"},
		{"詳細・出品者", "GET", "/items/" + target, "", "tok-s", 200, ""},
		{"詳細・管理者", "GET", "/items/" + target, "", "tok-a", 200, ""},
		{"購入", "POST", "/items/" + target + "/purchase", "", "tok-o", 404, "not_found"},
		{"やりとり", "POST", "/items/" + target + "/conversations", "", "tok-o", 404, "not_found"},
		{"いいね", "PUT", "/items/" + target + "/like", "", "tok-o", 404, "not_found"},
		{"いいねの取り消し", "DELETE", "/items/" + target + "/like", "", "tok-s", 404, "not_found"},
		{"要確認の商品は買える", "POST", "/items/" + review + "/purchase", "", "tok-o", 201, ""},
	}
	for _, s := range steps {
		rec := doAuth(h, s.method, s.path, s.body, s.token)
		if rec.Code != s.want || errCode(rec) != s.code {
			t.Fatalf("%s: want %d %q, got %d %s", s.name, s.want, s.code, rec.Code, rec.Body)
		}
	}

	// 一覧に非公開は出ない。要確認は出るが、理由はほかの人には見せない
	var items []model.Item
	rec := doAuth(h, "GET", "/items", "", "tok-o")
	if json.Unmarshal(rec.Body.Bytes(), &items) != nil || len(items) != 1 || items[0].Id != review ||
		items[0].ModerationStatus != "review" || items[0].ModerationReason != "" {
		t.Fatalf("一覧（ほかの人）: %s", rec.Body)
	}
	rec = doAuth(h, "GET", "/items/"+review, "", "tok-s")
	if !strings.Contains(rec.Body.String(), `"moderation_reason":"医薬品かもしれません"`) {
		t.Fatalf("出品者には理由を見せる: %s", rec.Body)
	}
	rec = doAuth(h, "GET", "/items/"+target, "", "tok-a")
	if !strings.Contains(rec.Body.String(), `"moderation_status":"hidden","moderation_reason":"写真が不適切"`) {
		t.Fatalf("管理者には非公開の理由（メモで上書き）を見せる: %s", rec.Body)
	}

	// 管理画面の一覧
	rec = doAuth(h, "GET", "/admin/items?status=hidden", "", "tok-a")
	if json.Unmarshal(rec.Body.Bytes(), &items) != nil || len(items) != 1 || items[0].Id != target || items[0].ModerationReason != "写真が不適切" {
		t.Fatalf("GET /admin/items?status=hidden: %s", rec.Body)
	}
	rec = doAuth(h, "GET", "/admin/items", "", "tok-a")
	if json.Unmarshal(rec.Body.Bytes(), &items) != nil || len(items) != 1 || items[0].Id != review {
		t.Fatalf("GET /admin/items（既定は review）: %s", rec.Body)
	}

	// 公開に戻す（メモなし → 理由はそのまま）と、また一覧に出る
	rec = doAuth(h, "PUT", "/admin/items/"+target+"/moderation", `{"status":"ok"}`, "tok-a")
	var it model.Item
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &it) != nil || it.ModerationStatus != "ok" || it.ModerationReason != "写真が不適切" {
		t.Fatalf("公開に戻す: %d %s", rec.Code, rec.Body)
	}
	if rec := doAuth(h, "GET", "/items/"+target, "", ""); rec.Code != 200 {
		t.Fatalf("公開に戻したら未ログインでも見える: %d", rec.Code)
	}
}

// ---- いいね ----

func TestLikesHTTP(t *testing.T) {
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	lim := usecase.NewLimiter(usecase.LikePerMinute, time.Minute)
	lim.Now = func() time.Time { return now }
	h, _, members := newAppHandler(nil, lim)
	setupUsers(t, h, members)
	a := createItem(t, h, "教科書").Id
	b := createItem(t, h, "机").Id

	steps := []struct {
		name, method, path, token string
		want                      int
		code                      string
		body                      string // 200 のときの本文
	}{
		{"ログインなし", "PUT", "/items/" + a + "/like", "", 401, "unauthorized", ""},
		{"未登録", "PUT", "/items/" + a + "/like", "tok-new", 403, "profile_required", ""},
		{"無い商品", "PUT", "/items/NOPE/like", "tok-o", 404, "not_found", ""},
		{"いいね", "PUT", "/items/" + a + "/like", "tok-o", 200, "", `{"liked":true,"like_count":1}`},
		{"2回目も成功", "PUT", "/items/" + a + "/like", "tok-o", 200, "", `{"liked":true,"like_count":1}`},
		{"自分の商品にも", "PUT", "/items/" + a + "/like", "tok-s", 200, "", `{"liked":true,"like_count":2}`},
		{"別の商品", "PUT", "/items/" + b + "/like", "tok-o", 200, "", `{"liked":true,"like_count":1}`},
		{"取り消し", "DELETE", "/items/" + a + "/like", "tok-s", 200, "", `{"liked":false,"like_count":1}`},
		{"付いていない取り消しも成功", "DELETE", "/items/" + a + "/like", "tok-s", 200, "", `{"liked":false,"like_count":1}`},
		{"いいねの一覧・ログインなし", "GET", "/me/likes", "", 401, "unauthorized", ""},
	}
	for _, s := range steps {
		rec := doAuth(h, s.method, s.path, "", s.token)
		if rec.Code != s.want || errCode(rec) != s.code || (s.body != "" && strings.TrimSpace(rec.Body.String()) != s.body) {
			t.Fatalf("%s: want %d %q %s, got %d %s", s.name, s.want, s.code, s.body, rec.Code, rec.Body)
		}
	}

	// 一覧・詳細の like_count と liked（ログインは任意。無効なトークンでも 200 で未ログイン扱い）
	var items []model.Item
	for _, c := range []struct {
		token string
		liked bool
	}{{"", false}, {"garbage", false}, {"tok-s", false}, {"tok-o", true}} {
		rec := doAuth(h, "GET", "/items", "", c.token)
		if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &items) != nil || len(items) != 2 {
			t.Fatalf("GET /items (%q): %d %s", c.token, rec.Code, rec.Body)
		}
		for _, it := range items {
			if it.Id == a && (it.LikeCount != 1 || it.Liked != c.liked) {
				t.Fatalf("GET /items (%q): %+v", c.token, it)
			}
		}
		var one model.Item
		rec = doAuth(h, "GET", "/items/"+a, "", c.token)
		if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &one) != nil || one.LikeCount != 1 || one.Liked != c.liked {
			t.Fatalf("GET /items/{id} (%q): %d %s", c.token, rec.Code, rec.Body)
		}
	}
	if rec := do(h, "GET", "/items/"+a, "", ""); !strings.Contains(rec.Body.String(), `"like_count":1,"liked":false`) {
		t.Fatalf("Item の JSON に like_count と liked: %s", rec.Body)
	}

	// 自分のいいね（いいねした新しい順）
	rec := doAuth(h, "GET", "/me/likes", "", "tok-o")
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &items) != nil || len(items) != 2 || items[0].Id != b || items[1].Id != a || !items[0].Liked {
		t.Fatalf("GET /me/likes: %d %s", rec.Code, rec.Body)
	}
	if rec := doAuth(h, "GET", "/me/likes", "", "tok-a"); strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("0件は [] を返す: %s", rec.Body)
	}

	// 回数制限：1人1分60回（いいねと取り消しを合わせて）
	for i := 0; i < usecase.LikePerMinute; i++ {
		doAuth(h, "PUT", "/items/"+a+"/like", "", "tok-a")
	}
	if rec := doAuth(h, "DELETE", "/items/"+a+"/like", "", "tok-a"); rec.Code != 429 || errCode(rec) != "too_many" {
		t.Fatalf("呼びすぎ: %d %s", rec.Code, rec.Body)
	}
	if rec := doAuth(h, "OPTIONS", "/items/"+a+"/like", "", ""); rec.Code != 204 {
		t.Fatalf("プリフライト: %d", rec.Code)
	}
}
