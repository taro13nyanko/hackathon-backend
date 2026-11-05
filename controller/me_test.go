package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hackathon-backend/auth"
	"hackathon-backend/model"
)

type fakeVerifier struct{}

func (fakeVerifier) Verify(_ context.Context, token string) (string, error) {
	if uid, ok := strings.CutPrefix(token, "tok-"); ok {
		return uid, nil
	}
	return "", auth.ErrInvalidToken
}

type memMemberStore struct{ members []model.Member }

func (m *memMemberStore) FindByFirebaseUID(uid string) (model.Member, error) {
	for _, x := range m.members {
		if x.FirebaseUID == uid {
			return x, nil
		}
	}
	return model.Member{}, model.ErrNotFound
}
func (m *memMemberStore) Insert(x model.Member) error {
	if _, err := m.FindByFirebaseUID(x.FirebaseUID); err == nil {
		return model.ErrAlreadyRegistered
	}
	m.members = append(m.members, x)
	return nil
}

func doAuth(h http.Handler, method, target, body, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func errCode(rec *httptest.ResponseRecorder) string {
	var e struct{ Error string }
	json.Unmarshal(rec.Body.Bytes(), &e)
	return e.Error
}

func TestMeFlow(t *testing.T) {
	h := newItemHandler()
	steps := []struct {
		name, method, path, body, token string
		want                            int
		code                            string
	}{
		{"トークンなし", "GET", "/me", "", "", 401, "unauthorized"},
		{"無効なトークン", "GET", "/me", "", "garbage", 401, "unauthorized"},
		{"未登録", "GET", "/me", "", "tok-u1", 404, "not_found"},
		{"未登録で出品", "POST", "/items", `{"title":"a","price":1000}`, "tok-u1", 403, "profile_required"},
		{"表示名が空", "POST", "/me", `{"display_name":"  "}`, "tok-u1", 400, "invalid_input"},
		{"壊れたJSON", "POST", "/me", `{"display_name":`, "tok-u1", 400, "invalid_input"},
		{"登録", "POST", "/me", `{"display_name":"ota"}`, "tok-u1", 201, ""},
		{"もう一度登録", "POST", "/me", `{"display_name":"ota2"}`, "tok-u1", 409, "already_registered"},
		{"登録済み", "GET", "/me", "", "tok-u1", 200, ""},
		{"安すぎる", "POST", "/items", `{"title":"a","price":100}`, "tok-u1", 400, "invalid_input"},
	}
	for _, s := range steps {
		rec := doAuth(h, s.method, s.path, s.body, s.token)
		if rec.Code != s.want || errCode(rec) != s.code {
			t.Fatalf("%s: want %d %q, got %d %s", s.name, s.want, s.code, rec.Code, rec.Body)
		}
	}
	rec := doAuth(h, "GET", "/me", "", "tok-u1")
	if !strings.Contains(rec.Body.String(), `"display_name":"ota"`) || strings.Contains(rec.Body.String(), "u1") {
		t.Fatalf("表示名を返し、firebase uid は出さない: %s", rec.Body)
	}
}
