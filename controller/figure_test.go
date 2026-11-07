package controller

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"testing"

	"hackathon-backend/usecase"
)

type fakeImgGen struct {
	out string
	err error
}

func (f *fakeImgGen) GenerateFromImage(context.Context, string, string, string, []byte) (string, error) {
	return f.out, f.err
}

func TestFigure(t *testing.T) {
	members := &usecase.MemberUsecase{Store: &memMemberStore{}}
	a := &Auth{Verifier: fakeVerifier{}, Members: members}
	gen := &fakeImgGen{out: `{"name":"本","parts":[{"shape":"box","size":[1,2,0.5],"pos":[0,1,0],"color":"#3355aa"}],"face":{"part":0,"style":"smile"}}`}
	mux := http.NewServeMux()
	(&MeController{Usecase: members, Auth: a}).Register(mux)
	(&FigureController{Usecase: &usecase.FigureUsecase{Gen: gen}, Auth: a}).Register(mux)
	h := CORS("", mux)
	doAuth(h, "POST", "/me", `{"display_name":"ota"}`, "tok-u")

	png := base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\n0000"))
	body := `{"image":"data:image/png;base64,` + png + `","name":"教科書"}`
	if rec := doAuth(h, "POST", "/figures", body, ""); rec.Code != 401 {
		t.Fatalf("ログインなし: %d", rec.Code)
	}
	rec := doAuth(h, "POST", "/figures", body, "tok-u")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"shape":"box"`) || !strings.Contains(rec.Body.String(), `"figure":`) {
		t.Fatalf("模型: %d %s", rec.Code, rec.Body)
	}
	if rec := doAuth(h, "POST", "/figures", `{"image":"%%%"}`, "tok-u"); rec.Code != 400 {
		t.Fatalf("base64 でない: %d", rec.Code)
	}
	gen.out = "だめ"
	if rec := doAuth(h, "POST", "/figures", body, "tok-u"); rec.Code != 502 || errCode(rec) != "ai_failed" {
		t.Fatalf("読めない出力: %d %s", rec.Code, rec.Body)
	}
	gen.err = errors.New("boom")
	if rec := doAuth(h, "POST", "/figures", body, "tok-u"); rec.Code != 429 && rec.Code != 502 {
		t.Fatalf("Gemini の失敗: %d %s", rec.Code, rec.Body)
	}
	if rec := doAuth(h, "POST", "/figures", body, "tok-u"); rec.Code != 429 || errCode(rec) != "too_many" {
		t.Fatalf("呼びすぎ: %d %s", rec.Code, rec.Body)
	}
}
