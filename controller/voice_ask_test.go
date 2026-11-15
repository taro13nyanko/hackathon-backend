package controller

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"hackathon-backend/model"
	"hackathon-backend/usecase"
)

// newSearchHandler は一覧・音声検索・質問を、偽の DB と偽の Gemini でつなぐ（本番と同じく LimitBody も通す）。
func newSearchHandler(voiceGen, askGen *fakeImgGen, voiceLimit, askLimit *usecase.Limiter) (http.Handler, *memItemStore) {
	members := &usecase.MemberUsecase{Store: &memMemberStore{}}
	a := &Auth{Verifier: fakeVerifier{}, Members: members}
	store := &memItemStore{}
	mux := http.NewServeMux()
	(&MeController{Usecase: members, Auth: a}).Register(mux)
	(&ItemController{Usecase: &usecase.ItemUsecase{Store: store}, Auth: a}).Register(mux)
	(&VoiceController{Usecase: &usecase.VoiceUsecase{Gen: voiceGen, Limit: voiceLimit}, Auth: a}).Register(mux)
	(&AskController{Usecase: &usecase.AskUsecase{Items: store, Gen: askGen, Limit: askLimit}, Auth: a}).Register(mux)
	h := CORS("", LimitBody(mux))
	doAuth(h, "POST", "/me", `{"display_name":"ota"}`, "tok-u")
	return h, store
}

// ---- GET /items の値段の範囲 ----

func TestItemsPriceRangeHTTP(t *testing.T) {
	h, store := newSearchHandler(&fakeImgGen{}, &fakeImgGen{}, nil, nil)
	for i, p := range []int{500, 1000, 3000, 8000} {
		store.items = append(store.items, model.Item{Id: "P" + string(rune('0'+i)), Title: "本", Price: p, ModerationStatus: model.ModerationOK})
	}
	cases := []struct {
		name   string
		target string
		want   int
		count  int
	}{
		{"指定なし", "/items", 200, 4},
		{"以下", "/items?max_price=3000", 200, 3},
		{"以上", "/items?min_price=1000", 200, 3},
		{"範囲", "/items?min_price=700&max_price=3000", 200, 2},
		{"逆でも入れ替える", "/items?min_price=3000&max_price=700", 200, 2},
		{"0 や空は指定なし", "/items?min_price=0&max_price=", 200, 4},
		{"q と組み合わせる", "/items?q=%E6%9C%AC&max_price=999", 200, 1}, // 本
		{"数字でなければ 400", "/items?min_price=abc", 400, 0},
		{"小数は 400", "/items?max_price=1.5", 400, 0},
		{"負は 400", "/items?max_price=-1", 400, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := do(h, "GET", c.target, "", "")
			if rec.Code != c.want {
				t.Fatalf("want %d, got %d %s", c.want, rec.Code, rec.Body)
			}
			if c.want != 200 {
				if errCode(rec) != "invalid_input" {
					t.Errorf("エラーコード: %s", rec.Body)
				}
				return
			}
			var items []model.Item
			if json.Unmarshal(rec.Body.Bytes(), &items) != nil || len(items) != c.count {
				t.Errorf("want %d 件, got %s", c.count, rec.Body)
			}
		})
	}
}

// ---- POST /search/voice ----

func TestVoiceSearchHTTP(t *testing.T) {
	gen := &fakeImgGen{out: `{"transcript":"三千円以下の教科書","q":"教科書","min_price":0,"max_price":3000}`}
	h, _ := newSearchHandler(gen, &fakeImgGen{}, nil, nil)
	audio := base64.StdEncoding.EncodeToString([]byte("OggS0000"))
	body := `{"audio":"data:audio/webm;codecs=opus;base64,` + audio + `","mime":"audio/webm;codecs=opus"}`

	if rec := doAuth(h, "POST", "/search/voice", body, ""); rec.Code != 401 {
		t.Fatalf("ログインなしは 401: %d", rec.Code)
	}
	if rec := doAuth(h, "POST", "/search/voice", body, "tok-nobody"); rec.Code != 403 || errCode(rec) != "profile_required" {
		t.Fatalf("表示名の登録なしは 403 profile_required: %d %s", rec.Code, rec.Body)
	}
	rec := doAuth(h, "POST", "/search/voice", body, "tok-u")
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"transcript":"三千円以下の教科書","q":"教科書","min_price":0,"max_price":3000}` {
		t.Fatalf("200 と検索の条件: %d %s", rec.Code, rec.Body)
	}

	bad := []struct{ name, body string }{
		{"JSON でない", `{`},
		{"base64 でない", `{"audio":"%%%","mime":"audio/webm"}`},
		{"一覧に無い mime", `{"audio":"` + audio + `","mime":"video/mp4"}`},
		{"音声が空", `{"audio":"","mime":"audio/webm"}`},
		{"本文の上限を超える", `{"audio":"` + strings.Repeat("A", VoiceBodyMax) + `","mime":"audio/webm"}`},
		{"音声が 1MB を超える", `{"audio":"` + base64.StdEncoding.EncodeToString(make([]byte, usecase.VoiceAudioMax+3)) + `","mime":"audio/webm"}`},
	}
	for _, c := range bad {
		if rec := doAuth(h, "POST", "/search/voice", c.body, "tok-u"); rec.Code != 400 || errCode(rec) != "invalid_input" {
			t.Errorf("%s は 400 invalid_input: %d %s", c.name, rec.Code, rec.Body)
		}
	}
	// 1MB ちょうどの音声は、共通の本文の上限（256KB）に止められずに通る
	big := `{"audio":"` + base64.StdEncoding.EncodeToString(make([]byte, usecase.VoiceAudioMax)) + `","mime":"audio/wav"}`
	if rec := doAuth(h, "POST", "/search/voice", big, "tok-u"); rec.Code != 200 {
		t.Fatalf("1MB の音声は通る: %d %s", rec.Code, rec.Body)
	}

	gen.err = errors.New("gemini down")
	if rec := doAuth(h, "POST", "/search/voice", body, "tok-u"); rec.Code != 502 || errCode(rec) != "ai_failed" {
		t.Fatalf("Gemini の失敗は 502 ai_failed: %d %s", rec.Code, rec.Body)
	}
	gen.err, gen.out = nil, "えっと"
	if rec := doAuth(h, "POST", "/search/voice", body, "tok-u"); rec.Code != 502 || errCode(rec) != "ai_failed" {
		t.Fatalf("読めない出力は 502 ai_failed: %d %s", rec.Code, rec.Body)
	}

	// 回数の上限
	h, _ = newSearchHandler(&fakeImgGen{out: `{"transcript":"本","q":"本","min_price":0,"max_price":0}`}, &fakeImgGen{}, usecase.NewLimiter(1, time.Minute), nil)
	doAuth(h, "POST", "/search/voice", body, "tok-u")
	if rec := doAuth(h, "POST", "/search/voice", body, "tok-u"); rec.Code != 429 || errCode(rec) != "too_many" {
		t.Fatalf("回数の上限は 429 too_many: %d %s", rec.Code, rec.Body)
	}
}

// ---- POST /items/{id}/ask ----

func TestAskHTTP(t *testing.T) {
	gen := &fakeImgGen{out: `{"answer":"駒場で手渡しできます[1][7]。","used":[1,7]}`}
	h, store := newSearchHandler(&fakeImgGen{}, gen, nil, nil)
	seller := model.Seller{Id: "S1", DisplayName: "ota"}
	store.items = append(store.items,
		model.Item{Id: "X1", Title: "線形代数の教科書", Price: 1200, Status: model.ItemOnSale, Seller: &seller,
			Description: "駒場で手渡しできます。", ModerationStatus: model.ModerationOK},
		model.Item{Id: "H1", Title: "非公開", Price: 500, Status: model.ItemOnSale, Seller: &seller, ModerationStatus: model.ModerationHidden},
	)
	body := `{"question":"受け渡しはどこですか"}`

	if rec := doAuth(h, "POST", "/items/X1/ask", body, ""); rec.Code != 401 {
		t.Fatalf("ログインなしは 401: %d", rec.Code)
	}
	rec := doAuth(h, "POST", "/items/X1/ask", body, "tok-u")
	want := `{"answer":"駒場で手渡しできます[1]。","sources":[{"id":1,"kind":"item","title":"この商品の説明","text":"商品名：線形代数の教科書。価格：1200円。状態：出品中。出品者：ota。説明：駒場で手渡しできます。"}],"grounded":true}`
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != want {
		t.Fatalf("200 と答え・根拠: %d %s", rec.Code, rec.Body)
	}

	gen.out = `{"answer":"この商品の情報からは分かりません。出品者にメッセージで聞いてみてください。","used":[]}`
	rec = doAuth(h, "POST", "/items/X1/ask", `{"question":"色は？"}`, "tok-u")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"sources":[],"grounded":false`) {
		t.Fatalf("根拠が無ければ sources は空の配列、grounded=false: %d %s", rec.Code, rec.Body)
	}

	cases := []struct {
		name, path, body string
		want             int
		code             string
	}{
		{"空の質問", "/items/X1/ask", `{"question":" "}`, 400, "invalid_input"},
		{"201字", "/items/X1/ask", `{"question":"` + strings.Repeat("あ", 201) + `"}`, 400, "invalid_input"},
		{"JSON でない", "/items/X1/ask", `{`, 400, "invalid_input"},
		{"非公開の商品", "/items/H1/ask", body, 404, "not_found"},
		{"無い商品", "/items/NOPE/ask", body, 404, "not_found"},
	}
	for _, c := range cases {
		if rec := doAuth(h, "POST", c.path, c.body, "tok-u"); rec.Code != c.want || errCode(rec) != c.code {
			t.Errorf("%s: want %d %s, got %d %s", c.name, c.want, c.code, rec.Code, rec.Body)
		}
	}

	gen.err = errors.New("gemini down")
	if rec := doAuth(h, "POST", "/items/X1/ask", body, "tok-u"); rec.Code != 502 || errCode(rec) != "ai_failed" {
		t.Fatalf("Gemini の失敗は 502 ai_failed: %d %s", rec.Code, rec.Body)
	}
	gen.err, gen.out = nil, "はい"
	if rec := doAuth(h, "POST", "/items/X1/ask", body, "tok-u"); rec.Code != 502 || errCode(rec) != "ai_failed" {
		t.Fatalf("読めない出力は 502 ai_failed: %d %s", rec.Code, rec.Body)
	}

	// 回数の上限
	h, store = newSearchHandler(&fakeImgGen{}, &fakeImgGen{out: `{"answer":"はい[1]","used":[1]}`}, nil, usecase.NewLimiter(1, time.Minute))
	store.items = append(store.items, model.Item{Id: "X1", Title: "本", Price: 300, ModerationStatus: model.ModerationOK})
	doAuth(h, "POST", "/items/X1/ask", body, "tok-u")
	if rec := doAuth(h, "POST", "/items/X1/ask", body, "tok-u"); rec.Code != 429 || errCode(rec) != "too_many" {
		t.Fatalf("回数の上限は 429 too_many: %d %s", rec.Code, rec.Body)
	}
}
