package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hackathon-backend/model"
	"hackathon-backend/usecase"
)

type memStore struct{ users []model.User }

func (m *memStore) FindByName(n string) ([]model.User, error) {
	out := []model.User{}
	for _, u := range m.users {
		if u.Name == n {
			out = append(out, u)
		}
	}
	return out, nil
}
func (m *memStore) FindAll() ([]model.User, error) {
	return append([]model.User{}, m.users...), nil
}
func (m *memStore) Insert(u model.User) error { m.users = append(m.users, u); return nil }

func newHandler(allowed string) http.Handler {
	c := &UserController{Usecase: &usecase.UserUsecase{Store: &memStore{}}}
	mux := http.NewServeMux()
	mux.Handle("/user", c)
	mux.HandleFunc("/users", c.ListHandler)
	return CORS(allowed, mux)
}

func do(h http.Handler, method, target, body, origin string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestPostThenGet(t *testing.T) {
	h := newHandler("")
	rec := do(h, "POST", "/user", `{"name":"hanako","age":20}`, "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"id"`) {
		t.Fatalf("POST: %d %s", rec.Code, rec.Body)
	}
	rec = do(h, "GET", "/user?name=hanako", "", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"hanako"`) {
		t.Fatalf("GET: %d %s", rec.Code, rec.Body)
	}
	if rec := do(h, "GET", "/user?name=nobody", "", ""); rec.Body.String() != "[]" {
		t.Fatalf("no match should be [], got %s", rec.Body)
	}
}

func TestBadRequests(t *testing.T) {
	h := newHandler("")
	for _, c := range []struct{ m, url, body string }{
		{"POST", "/user", `{"name":"x","age":10}`},
		{"POST", "/user", `not json`},
		{"GET", "/user", ""},
	} {
		if rec := do(h, c.m, c.url, c.body, ""); rec.Code != 400 {
			t.Errorf("%s %s %q: want 400, got %d", c.m, c.url, c.body, rec.Code)
		}
	}
	if rec := do(h, "DELETE", "/user", "", ""); rec.Code != 405 {
		t.Errorf("DELETE: want 405, got %d", rec.Code)
	}
}

func TestCORS(t *testing.T) {
	h := newHandler("https://app.vercel.app/, http://localhost:5173")

	rec := do(h, "OPTIONS", "/user", "", "https://app.vercel.app")
	if rec.Code != 204 || rec.Header().Get("Access-Control-Allow-Origin") != "https://app.vercel.app" {
		t.Fatalf("preflight: %d %q", rec.Code, rec.Header().Get("Access-Control-Allow-Origin"))
	}
	rec = do(h, "GET", "/user?name=a", "", "https://evil.example")
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("unlisted origin must not be allowed, got %q", got)
	}
	rec = do(newHandler(""), "GET", "/user?name=a", "", "https://any.example")
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("unset ALLOWED_ORIGINS should allow *, got %q", got)
	}
}

func TestListUsers(t *testing.T) {
	h := newHandler("")
	if rec := do(h, "GET", "/users", "", ""); rec.Code != 200 || rec.Body.String() != "[]" {
		t.Fatalf("empty: %d %s", rec.Code, rec.Body)
	}
	do(h, "POST", "/user", `{"name":"hanako","age":20}`, "")
	do(h, "POST", "/user", `{"name":"taro","age":30}`, "")
	rec := do(h, "GET", "/users", "", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "hanako") || !strings.Contains(rec.Body.String(), "taro") {
		t.Fatalf("list: %d %s", rec.Code, rec.Body)
	}
	if rec := do(h, "POST", "/users", "", ""); rec.Code != 405 {
		t.Fatalf("POST /users: want 405, got %d", rec.Code)
	}
}
