package controller

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"hackathon-backend/model"
	"hackathon-backend/usecase"
)

// 既読（PUT /conversations/{id}/read）と未読の合計（GET /me/unread）の受け口。
// 数え方と参加者の確かめは usecase のテスト（usecase/dm_test.go）で見る。
func TestDMReadRoutes(t *testing.T) {
	h, _, _ := newAppHandler(nil, nil)
	if rec := doAuth(h, http.MethodPost, "/me", `{"display_name":"ota"}`, "tok-uid-1"); rec.Code != http.StatusCreated {
		t.Fatalf("POST /me: %d %s", rec.Code, rec.Body)
	}

	cases := []struct {
		name, method, target, body, token string
		code                              int
		want                              string
	}{
		{"未読の合計", "GET", "/me/unread", "", "tok-uid-1", 200, `{"count":0}`},
		{"未読の合計はログインが要る", "GET", "/me/unread", "", "", 401, "unauthorized"},
		{"表示名の登録が要る", "GET", "/me/unread", "", "tok-uid-2", 403, "profile_required"},
		{"無いやりとりは 404（ボディ省略）", "PUT", "/conversations/C9/read", "", "tok-uid-1", 404, "not_found"},
		{"無いやりとりは 404（ID つき）", "PUT", "/conversations/C9/read", `{"last_message_id":"01J"}`, "tok-uid-1", 404, "not_found"},
		{"壊れた JSON は 400", "PUT", "/conversations/C9/read", `{`, "tok-uid-1", 400, "invalid_input"},
		{"既読はログインが要る", "PUT", "/conversations/C9/read", "", "", 401, "unauthorized"},
	}
	for _, tc := range cases {
		rec := doAuth(h, tc.method, tc.target, tc.body, tc.token)
		if rec.Code != tc.code || !strings.Contains(rec.Body.String(), tc.want) {
			t.Errorf("%s: %d %s", tc.name, rec.Code, rec.Body)
		}
	}
}

// readDMStore はやりとり C1（買い手 buyer・出品者 seller）と、2人の既読の位置だけを持つ。
type readDMStore struct {
	memDMStore
	buyer, seller string
	reads         map[string]string // 人 → 既読の位置
}

func (s *readDMStore) Participants(id string) (string, string, error) {
	if id != "C1" {
		return "", "", model.ErrNotFound
	}
	return s.buyer, s.seller, nil
}
func (s *readDMStore) LastRead(_, user string) (string, error) { return s.reads[user], nil }

// 相手の既読の位置（GET /conversations/{id}/read）の受け口。
func TestDMPeerReadRoute(t *testing.T) {
	members := &usecase.MemberUsecase{Store: &memMemberStore{}}
	a := &Auth{Verifier: fakeVerifier{}, Members: members}
	store := &readDMStore{}
	mux := http.NewServeMux()
	(&MeController{Usecase: members, Auth: a}).Register(mux)
	(&DMController{Usecase: &usecase.DMUsecase{Store: store}, Auth: a}).Register(mux)
	h := CORS("", mux)

	ids := map[string]string{}
	for _, uid := range []string{"b", "s", "x"} {
		rec := doAuth(h, http.MethodPost, "/me", `{"display_name":"`+uid+`"}`, "tok-"+uid)
		var m model.Member
		if rec.Code != http.StatusCreated || json.Unmarshal(rec.Body.Bytes(), &m) != nil || m.Id == "" {
			t.Fatalf("POST /me %s: %d %s", uid, rec.Code, rec.Body)
		}
		ids[uid] = m.Id
	}
	store.buyer, store.seller = ids["b"], ids["s"]
	store.reads = map[string]string{ids["b"]: "01B", ids["s"]: "01C"}

	cases := []struct {
		name, target, token string
		code                int
		want                string
	}{
		{"買い手には出品者の位置", "/conversations/C1/read", "tok-b", 200, `{"peer_last_read_message_id":"01C"}`},
		{"出品者には買い手の位置", "/conversations/C1/read", "tok-s", 200, `{"peer_last_read_message_id":"01B"}`},
		{"参加者以外は 403", "/conversations/C1/read", "tok-x", 403, "forbidden"},
		{"無いやりとりは 404", "/conversations/C9/read", "tok-b", 404, "not_found"},
		{"ログインが要る", "/conversations/C1/read", "", 401, "unauthorized"},
	}
	for _, tc := range cases {
		rec := doAuth(h, http.MethodGet, tc.target, "", tc.token)
		if rec.Code != tc.code || !strings.Contains(rec.Body.String(), tc.want) {
			t.Errorf("%s: %d %s", tc.name, rec.Code, rec.Body)
		}
	}

	// 相手がまだ読んでいなければ空
	delete(store.reads, ids["s"])
	if rec := doAuth(h, http.MethodGet, "/conversations/C1/read", "", "tok-b"); rec.Code != 200 || !strings.Contains(rec.Body.String(), `{"peer_last_read_message_id":""}`) {
		t.Errorf("まだ読んでいない: %d %s", rec.Code, rec.Body)
	}
}
