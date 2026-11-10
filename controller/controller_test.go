package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func do(h http.Handler, method, target, body, origin string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func pingMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ping", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("pong")) })
	return mux
}

func TestCORS(t *testing.T) {
	h := CORS("https://app.vercel.app/, http://localhost:5173", pingMux())

	rec := do(h, "OPTIONS", "/ping", "", "https://app.vercel.app")
	if rec.Code != 204 || rec.Header().Get("Access-Control-Allow-Origin") != "https://app.vercel.app" {
		t.Fatalf("preflight: %d %q", rec.Code, rec.Header().Get("Access-Control-Allow-Origin"))
	}
	rec = do(h, "GET", "/ping", "", "https://evil.example")
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("unlisted origin must not be allowed, got %q", got)
	}
	rec = do(CORS("", pingMux()), "GET", "/ping", "", "https://any.example")
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("unset ALLOWED_ORIGINS should allow *, got %q", got)
	}
}

// 課題用の /user・/users は消した（だれでも書き込めたため）。
func TestCurriculumUserEndpointsRemoved(t *testing.T) {
	h := newItemHandler()
	for _, target := range []string{"/user", "/users", "/user?name=hanako"} {
		if rec := do(h, "GET", target, "", ""); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s: want 404, got %d", target, rec.Code)
		}
	}
	if rec := do(h, "POST", "/user", `{"name":"hanako","age":20}`, ""); rec.Code != http.StatusNotFound {
		t.Errorf("POST /user: want 404, got %d", rec.Code)
	}
}

func TestLimitBody(t *testing.T) {
	var got int
	echo := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		got = len(b)
		if err != nil {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
		}
	})
	h := LimitBody(echo)
	big := strings.Repeat("a", BodyMax+1)
	if rec := do(h, "POST", "/items", big, ""); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("上限を超える本文は読めない: %d (%d bytes)", rec.Code, got)
	}
	if rec := do(h, "POST", "/items", strings.Repeat("a", BodyMax), ""); rec.Code != 200 || got != BodyMax {
		t.Fatalf("上限ちょうどは読める: %d %d", rec.Code, got)
	}
	// 画像と模型づくりは、それぞれのハンドラが自分の上限を付ける
	for _, p := range []string{"/images", "/figures"} {
		if rec := do(h, "POST", p, big, ""); rec.Code != 200 || got != len(big) {
			t.Fatalf("%s は共通の上限の対象外: %d %d", p, rec.Code, got)
		}
	}
}
