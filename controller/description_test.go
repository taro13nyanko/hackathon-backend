package controller

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"hackathon-backend/usecase"
)

type fakeGen struct{ err error }

func (f *fakeGen) Generate(context.Context, string, string) (string, error) {
	return "下書きです。", f.err
}

func TestDescription(t *testing.T) {
	members := &usecase.MemberUsecase{Store: &memMemberStore{}}
	a := &Auth{Verifier: fakeVerifier{}, Members: members}
	gen := &fakeGen{}
	mux := http.NewServeMux()
	(&ItemController{Usecase: &usecase.ItemUsecase{Store: &memItemStore{}}, Auth: a}).Register(mux) // 同じ /items の下に並べても衝突しない
	(&MeController{Usecase: members, Auth: a}).Register(mux)
	(&DescriptionController{Usecase: &usecase.DescriptionUsecase{Gen: gen}, Auth: a}).Register(mux)
	h := CORS("", mux)
	doAuth(h, "POST", "/me", `{"display_name":"ota"}`, "tok-u")

	if rec := doAuth(h, "POST", "/items/description", `{"title":"机"}`, ""); rec.Code != 401 {
		t.Fatalf("ログインなし: %d", rec.Code)
	}
	rec := doAuth(h, "POST", "/items/description", `{"title":"机","notes":"脚に傷"}`, "tok-u")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"description":"下書きです。"`) {
		t.Fatalf("下書き: %d %s", rec.Code, rec.Body)
	}
	if rec := doAuth(h, "POST", "/items/description", `{"title":""}`, "tok-u"); rec.Code != 400 {
		t.Fatalf("商品名なし: %d", rec.Code)
	}
	gen.err = errors.New("boom")
	if rec := doAuth(h, "POST", "/items/description", `{"title":"机"}`, "tok-u"); rec.Code != 502 || errCode(rec) != "ai_failed" {
		t.Fatalf("Gemini の失敗: %d %s", rec.Code, rec.Body)
	}
	for i := 0; i < usecase.DescPerMinute; i++ {
		doAuth(h, "POST", "/items/description", `{"title":"机"}`, "tok-u")
	}
	if rec := doAuth(h, "POST", "/items/description", `{"title":"机"}`, "tok-u"); rec.Code != 429 || errCode(rec) != "too_many" {
		t.Fatalf("呼びすぎ: %d %s", rec.Code, rec.Body)
	}
}
