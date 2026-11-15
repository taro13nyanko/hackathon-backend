package controller

import (
	"net/http"
	"strings"
	"testing"
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
