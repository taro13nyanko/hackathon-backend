package usecase

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"hackathon-backend/gemini"
	"hackathon-backend/model"
)

// fakeJSONGen は JSON 出力の呼び方（GenerateFromImages）を真似る偽の Gemini。渡されたものを覚える（画像・音声は0枚でもよい）。
type fakeJSONGen struct {
	system, prompt string
	images         []gemini.Image
	calls          int
	out            string
	err            error
}

func (f *fakeJSONGen) GenerateFromImages(_ context.Context, system, prompt string, imgs []gemini.Image) (string, error) {
	f.calls++
	f.system, f.prompt, f.images = system, prompt, imgs
	return f.out, f.err
}

var askSeller = model.Seller{Id: "S1", DisplayName: "ota"}

func askStore() *fakeItemStore {
	return &fakeItemStore{items: []model.Item{
		{Id: "X1", Title: "線形代数の教科書", Price: 1200, Status: model.ItemOnSale, Seller: &askSeller, ModerationStatus: model.ModerationReview,
			ModerationReason: "ひみつの理由",
			Description:      "東大出版会の教科書です。書き込みが少しあります。駒場キャンパスで手渡しできます。",
			FigureCaption:    "青い表紙の本の模型"},
		{Id: "X2", Title: "微積分の教科書", Price: 800, Status: model.ItemSold, Seller: &askSeller, ModerationStatus: model.ModerationOK, Description: "演習問題つき。"},
		{Id: "X3", Title: "非公開の品", Price: 500, Status: model.ItemOnSale, Seller: &askSeller, ModerationStatus: model.ModerationHidden},
		{Id: "H1", Title: "隠れた商品", Price: 500, Status: model.ItemOnSale, Seller: &askSeller, ModerationStatus: model.ModerationHidden},
	}}
}

func TestAsk(t *testing.T) {
	me := model.Member{Id: "B1", DisplayName: "buyer"}
	boom := errors.New("gemini down")
	cases := []struct {
		name         string
		item         string
		question     string
		out          string
		genErr       error
		wantErr      error
		wantCall     bool
		wantIds      []int
		wantGrounded bool
		wantAnswer   string
	}{
		{name: "根拠つきの答え（存在しない番号は消す・used に無い引用は足す）", item: "X1", question: "受け渡しの方法は？",
			out:      `{"answer":"駒場キャンパスで手渡しできます[1]。方法はメッセージで相談します[2][9]。","used":[1,7]}`,
			wantCall: true, wantIds: []int{1, 2}, wantGrounded: true, wantAnswer: "駒場キャンパスで手渡しできます[1]。方法はメッセージで相談します[2]。"},
		{name: "分からないときは grounded=false", item: "X1", question: "色は？",
			out:      `{"answer":"` + AskNoInfo + `","used":[]}`,
			wantCall: true, wantIds: []int{}, wantGrounded: false, wantAnswer: AskNoInfo},
		{name: "空の質問は 400", item: "X1", question: "   ", wantErr: ErrInvalidInput},
		{name: "201字は 400", item: "X1", question: strings.Repeat("あ", AskQuestionMax+1), wantErr: ErrInvalidInput},
		{name: "200字は通る", item: "X1", question: strings.Repeat("あ", AskQuestionMax), out: `{"answer":"はい[1]","used":[1]}`,
			wantCall: true, wantIds: []int{1}, wantGrounded: true, wantAnswer: "はい[1]"},
		{name: "非公開の商品は 404", item: "H1", question: "状態は？", wantErr: model.ErrNotFound},
		{name: "無い商品は 404", item: "NOPE", question: "状態は？", wantErr: model.ErrNotFound},
		{name: "ID が空は 400", item: "", question: "状態は？", wantErr: ErrInvalidInput},
		{name: "Gemini の失敗はそのまま返す", item: "X1", question: "状態は？", genErr: boom, wantErr: boom, wantCall: true},
		{name: "読めない出力は ErrAIOutput", item: "X1", question: "状態は？", out: "すみません", wantErr: ErrAIOutput, wantCall: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gen := &fakeJSONGen{out: c.out, err: c.genErr}
			uc := &AskUsecase{Items: askStore(), Gen: gen}
			res, err := uc.Ask(context.Background(), me, c.item, AskReq{Question: c.question})
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("err: got %v, want %v", err, c.wantErr)
			}
			if (gen.calls > 0) != c.wantCall {
				t.Fatalf("Gemini を呼ぶか: calls=%d want %v", gen.calls, c.wantCall)
			}
			if c.wantErr != nil {
				return
			}
			ids := []int{}
			for _, s := range res.Sources {
				ids = append(ids, s.Id)
				if s.Kind == "" || s.Title == "" || s.Text == "" || len([]rune(s.Text)) > AskSourceTextMax {
					t.Errorf("根拠の形: %+v", s)
				}
			}
			if !reflect.DeepEqual(ids, c.wantIds) || res.Grounded != c.wantGrounded || res.Answer != c.wantAnswer || res.Sources == nil {
				t.Errorf("got %+v", res)
			}
			if len(gen.images) != 0 || gen.system != askSystem {
				t.Errorf("画像は渡さず、決まった指示で呼ぶ")
			}
		})
	}
}

// Gemini に渡す断片：この商品の説明の先頭が [1]、質問に近い決まりが入る。非公開の商品と確認の理由は入れない。
func TestAskPrompt(t *testing.T) {
	gen := &fakeJSONGen{out: `{"answer":"相談して決めます[2]","used":[2]}`}
	uc := &AskUsecase{Items: askStore(), Gen: gen}
	res, err := uc.Ask(context.Background(), model.Member{Id: "B1"}, "X1", AskReq{Question: "  受け渡しの方法は？ "})
	if err != nil {
		t.Fatal(err)
	}
	p := gen.prompt
	if !strings.Contains(p, "[1]（この商品：この商品の説明）\n<<<\n商品名：線形代数の教科書。価格：1200円。状態：出品中。出品者：ota。説明：") {
		t.Errorf("[1] はこの商品の説明の先頭:\n%s", p)
	}
	if !strings.Contains(p, "（アプリの決まり：受け渡し）") || !strings.HasSuffix(p, "## 質問\n<<<\n受け渡しの方法は？\n>>>") {
		t.Errorf("質問に近い決まりと、前後の空白を除いた質問を渡す:\n%s", p)
	}
	if strings.Contains(p, "非公開の品") || strings.Contains(p, "隠れた商品") || strings.Contains(p, "ひみつの理由") {
		t.Errorf("非公開の商品・確認の理由を渡してはいけない:\n%s", p)
	}
	if strings.Count(p, "<<<") != strings.Count(p, ">>>") || strings.Count(p, "\n[") > AskTopK {
		t.Errorf("断片は %d 件まで、区切りで囲む:\n%s", AskTopK, p)
	}
	if len(res.Sources) != 1 || res.Sources[0].Id != 2 || res.Sources[0].Kind != ChunkRule || res.Sources[0].Title != "受け渡し" {
		t.Errorf("sources は used の断片だけ: %+v", res.Sources)
	}
}

func TestAskLimit(t *testing.T) {
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	limit := NewLimiter(AskPerMinute, time.Minute)
	limit.Now = func() time.Time { return now }
	gen := &fakeJSONGen{out: `{"answer":"はい[1]","used":[1]}`}
	uc := &AskUsecase{Items: askStore(), Gen: gen, Limit: limit}
	me := model.Member{Id: "B1"}
	for i := 0; i < AskPerMinute; i++ {
		if _, err := uc.Ask(context.Background(), me, "X1", AskReq{Question: "状態は？"}); err != nil {
			t.Fatalf("%d 回目: %v", i+1, err)
		}
	}
	if _, err := uc.Ask(context.Background(), me, "X1", AskReq{Question: "状態は？"}); !errors.Is(err, ErrTooMany) || gen.calls != AskPerMinute {
		t.Fatalf("1分に %d 回まで（超えたら Gemini を呼ばない）: %v calls=%d", AskPerMinute, err, gen.calls)
	}
	if _, err := uc.Ask(context.Background(), model.Member{Id: "B2"}, "X1", AskReq{Question: "状態は？"}); err != nil {
		t.Fatalf("回数は人ごと: %v", err)
	}
}

func TestBuildChunks(t *testing.T) {
	store := askStore()
	it := store.items[0]
	chunks := BuildChunks(it, store.items, AppRules)
	if chunks[0].Kind != ChunkItem || !strings.HasPrefix(chunks[0].Text, "商品名：線形代数の教科書。") || !strings.Contains(chunks[0].Text, "説明：東大出版会の教科書です。") {
		t.Errorf("0 番はこの商品の説明の先頭: %+v", chunks[0])
	}
	kinds := map[string]int{}
	for _, c := range chunks {
		kinds[c.Kind]++
		if c.Title == "" || c.Text == "" {
			t.Errorf("空の断片: %+v", c)
		}
		if strings.Contains(c.Text, "非公開の品") || strings.Contains(c.Text, "ひみつの理由") || c.Title == "線形代数の教科書" {
			t.Errorf("非公開の商品・確認の理由・この商品自身を「他の商品」に入れない: %+v", c)
		}
	}
	if kinds[ChunkFigure] != 1 || kinds[ChunkSellerItem] != 1 || kinds[ChunkRule] != len(AppRules) || kinds[ChunkItem] < 1 {
		t.Errorf("断片の種類: %v", kinds)
	}
	if len(AppRules) < 10 || len(AppRules) > 15 {
		t.Errorf("アプリの決まりは10〜15項目: %d", len(AppRules))
	}

	// 見本データ（出品者なし）・説明なし・模型なし
	sample := BuildChunks(model.Item{Id: "S", Title: "見本", Price: 300, Status: model.ItemOnSale}, nil, nil)
	if len(sample) != 1 || !strings.Contains(sample[0].Text, "出品者：なし（見本データ）") || !strings.HasSuffix(sample[0].Text, "説明：なし") {
		t.Errorf("見本データ: %+v", sample)
	}

	// 他の商品は新しい順に AskSellerItemsMax 件まで
	many := []model.Item{}
	for i := 0; i < AskSellerItemsMax+5; i++ {
		many = append(many, model.Item{Id: string(rune('a' + i)), Title: "他", Price: 300})
	}
	n := 0
	for _, c := range BuildChunks(it, many, nil) {
		if c.Kind == ChunkSellerItem {
			n++
		}
	}
	if n != AskSellerItemsMax {
		t.Errorf("他の商品は %d 件まで: %d", AskSellerItemsMax, n)
	}
}

func TestParseAnswer(t *testing.T) {
	long := strings.Repeat("あ", AskAnswerMax-2) + "[1]です"
	cases := []struct {
		name       string
		raw        string
		n          int
		wantAnswer string
		wantUsed   []int
		wantErr    error
	}{
		{"そのまま", `{"answer":"はい[1]","used":[1]}`, 3, "はい[1]", []int{1}, nil},
		{"コードブロックで囲まれていても読む", "```json\n{\"answer\":\"はい[2]\",\"used\":[2]}\n```", 3, "はい[2]", []int{2}, nil},
		{"used は実在の番号だけ・重複なし・小さい順", `{"answer":"はい","used":[3,1,3,0,-1,4]}`, 3, "はい", []int{1, 3}, nil},
		{"答えの中の実在しない番号は消す", `{"answer":"はい[0][4][2]","used":[]}`, 3, "はい[2]", []int{2}, nil},
		{"400字で切り、末尾の欠けた番号を消す", `{"answer":"` + long + `","used":[1]}`, 3, strings.Repeat("あ", AskAnswerMax-2), []int{1}, nil},
		{"used が null でも読む", `{"answer":"分かりません"}`, 3, "分かりません", []int{}, nil},
		{"答えが空は ErrAIOutput", `{"answer":"  ","used":[1]}`, 3, "", nil, ErrAIOutput},
		{"番号だけ（消すと空）は ErrAIOutput", `{"answer":"[9]","used":[]}`, 3, "", nil, ErrAIOutput},
		{"JSON でない", `はい`, 3, "", nil, ErrAIOutput},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			answer, used, err := ParseAnswer(c.raw, c.n)
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("err: got %v, want %v", err, c.wantErr)
			}
			if c.wantErr != nil {
				return
			}
			if answer != c.wantAnswer || !reflect.DeepEqual(used, c.wantUsed) {
				t.Errorf("got %q %v, want %q %v", answer, used, c.wantAnswer, c.wantUsed)
			}
		})
	}
}
