package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"hackathon-backend/model"
)

type fakeGen struct {
	system, prompt, out string
	err                 error
}

func (f *fakeGen) Generate(_ context.Context, system, prompt string) (string, error) {
	f.system, f.prompt = system, prompt
	return f.out, f.err
}

func TestDraft(t *testing.T) {
	gen := &fakeGen{out: "  線形代数の教科書です。\n書き込みが少しあります。 "}
	now := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	uc := &DescriptionUsecase{Gen: gen, Now: func() time.Time { return now }}
	me := model.Member{Id: "m1"}

	got, err := uc.Draft(context.Background(), me, DescriptionReq{Title: " 線形代数の教科書 ", Notes: "書き込み少し、駒場で手渡し"})
	if err != nil || got != "線形代数の教科書です。\n書き込みが少しあります。" {
		t.Fatalf("%q %v", got, err)
	}
	if !strings.Contains(gen.prompt, "「線形代数の教科書」") || !strings.Contains(gen.prompt, "駒場で手渡し") {
		t.Errorf("商品名とメモを渡すこと: %q", gen.prompt)
	}
	if !strings.Contains(gen.system, "メモに書かれた事実だけ") {
		t.Errorf("事実だけで書くよう指示すること")
	}

	for _, req := range []DescriptionReq{{Title: ""}, {Title: strings.Repeat("あ", ItemTitleMax+1)}, {Title: "a", Notes: strings.Repeat("あ", DescNotesMax+1)}} {
		if _, err := uc.Draft(context.Background(), me, req); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%d/%d 文字: %v", len([]rune(req.Title)), len([]rune(req.Notes)), err)
		}
	}

	gen.out = strings.Repeat("長", DescriptionCut+50)
	if got, _ := uc.Draft(context.Background(), model.Member{Id: "m2"}, DescriptionReq{Title: "a"}); len([]rune(got)) != DescriptionCut {
		t.Errorf("長すぎる出力は切る: %d", len([]rune(got)))
	}

	gen.err = errors.New("boom")
	if _, err := uc.Draft(context.Background(), model.Member{Id: "m3"}, DescriptionReq{Title: "a"}); err == nil {
		t.Errorf("Gemini のエラーを返すこと")
	}
}

func TestDraftRateLimit(t *testing.T) {
	now := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	uc := &DescriptionUsecase{Gen: &fakeGen{out: "ok"}, Now: func() time.Time { return now }}
	me := model.Member{Id: "m1"}
	for i := 0; i < DescPerMinute; i++ {
		if _, err := uc.Draft(context.Background(), me, DescriptionReq{Title: "a"}); err != nil {
			t.Fatal(i, err)
		}
	}
	if _, err := uc.Draft(context.Background(), me, DescriptionReq{Title: "a"}); !errors.Is(err, ErrTooMany) {
		t.Fatalf("%d 回目は ErrTooMany: %v", DescPerMinute+1, err)
	}
	if _, err := uc.Draft(context.Background(), model.Member{Id: "m2"}, DescriptionReq{Title: "a"}); err != nil {
		t.Fatalf("別の人は使える: %v", err)
	}
	now = now.Add(61 * time.Second)
	if _, err := uc.Draft(context.Background(), me, DescriptionReq{Title: "a"}); err != nil {
		t.Fatalf("1分たてば使える: %v", err)
	}
}
