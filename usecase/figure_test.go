package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"hackathon-backend/model"
)

type fakeImgGen struct {
	prompt, mime string
	out          string
	err          error
}

func (f *fakeImgGen) GenerateFromImage(_ context.Context, _, prompt, mime string, _ []byte) (string, error) {
	f.prompt, f.mime = prompt, mime
	return f.out, f.err
}

var pngHead = []byte("\x89PNG\r\n\x1a\n0000000000")

func TestFigureFromImage(t *testing.T) {
	gen := &fakeImgGen{out: "```json\n" + `{"name":"ランプ","caption":"ランプさん","parts":[{"shape":"cone","size":[0.4,1,0.9],"pos":[0,1.6,0],"color":"#FFD166"}],"face":{"part":0,"style":"sleepy","y":1.5}}` + "\n```"}
	uc := &FigureUsecase{Gen: gen}
	me := model.Member{Id: "m1"}
	f, err := uc.FromImage(context.Background(), me, pngHead, " デスクライト ")
	if err != nil || f.Name != "ランプ" || len(f.Parts) != 1 || f.Parts[0].Color != "#ffd166" || f.Face.Style != "sleepy" {
		t.Fatalf("%+v %v", f, err)
	}
	if gen.mime != "image/png" || !strings.Contains(gen.prompt, "「デスクライト」") {
		t.Errorf("画像の種類と商品名を渡すこと: %q %q", gen.mime, gen.prompt)
	}
	for _, img := range [][]byte{nil, []byte("hello, not an image"), make([]byte, FigureImageMax+1)} {
		if _, err := uc.FromImage(context.Background(), me, img, ""); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("画像でないものは ErrInvalidInput: %v", err)
		}
	}
	gen.out = "すみません"
	if _, err := uc.FromImage(context.Background(), me, pngHead, ""); !errors.Is(err, ErrAIOutput) {
		t.Errorf("読めない出力は ErrAIOutput: %v", err)
	}
}

func TestFigureRateLimit(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	uc := &FigureUsecase{Gen: &fakeImgGen{out: `{"parts":[{"shape":"sphere","size":[1],"pos":[0,1,0]}]}`}, Now: func() time.Time { return now }}
	me := model.Member{Id: "m1"}
	for i := 0; i < FigurePerMinute; i++ {
		if _, err := uc.FromImage(context.Background(), me, pngHead, ""); err != nil {
			t.Fatal(i, err)
		}
	}
	if _, err := uc.FromImage(context.Background(), me, pngHead, ""); !errors.Is(err, ErrTooMany) {
		t.Fatalf("呼びすぎ: %v", err)
	}
	now = now.Add(61 * time.Second)
	if _, err := uc.FromImage(context.Background(), me, pngHead, ""); err != nil {
		t.Fatalf("1分たてば使える: %v", err)
	}
}

func TestParseFigureCleans(t *testing.T) {
	raw := `{"name":"` + strings.Repeat("長", 30) + `","parts":[
	  {"shape":"star","size":[1],"pos":[0,0,0]},
	  {"shape":"box","size":[1,1],"pos":[0,0,0]},
	  {"shape":"box","size":[99,0,1],"pos":[0,99,0],"color":"red","round":3},
	  {"shape":"label","size":[1,0.3],"pos":[0,1,0.5],"text":"` + strings.Repeat("字", 20) + `","color":"#ffffff"},
	  {"shape":"label","size":[1,0.3],"pos":[0,1,0.5],"text":" "},
	  {"shape":"torus","size":[0.5,0.1],"pos":[0,0,0],"arc":0,"rot":[90,0,0]}
	],"face":{"part":2,"style":"angry","scale":10}}`
	f, err := ParseFigure(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len([]rune(f.Name)) != FigureNameMax {
		t.Errorf("名前を切る: %q", f.Name)
	}
	if len(f.Parts) != 3 {
		t.Fatalf("形が無い・size が足りない・文字の無い札は捨てる: %d 個 %+v", len(f.Parts), f.Parts)
	}
	b := f.Parts[0]
	if b.Size[0] != 4 || b.Size[1] != 0.005 || b.Pos[1] != 5 || b.Color != "#cccccc" || *b.Round != 0.5 {
		t.Errorf("範囲に収める: %+v round=%v", b, *b.Round)
	}
	if l := f.Parts[1]; len([]rune(l.Text)) != FigureLabelMax || l.Color != "" || l.Ink != "#222222" {
		t.Errorf("札: %+v", l)
	}
	if *f.Parts[2].Arc != 10 {
		t.Errorf("arc: %v", *f.Parts[2].Arc)
	}
	if f.Face == nil || f.Face.Part != 0 || f.Face.Style != "smile" || *f.Face.Scale != 2 {
		t.Errorf("顔の番号を付け直し、表情と大きさを直す: %+v", f.Face)
	}

	f, _ = ParseFigure(`{"parts":[{"shape":"label","size":[1,1],"pos":[0,0,0],"text":"a"}],"face":{"part":0}}`)
	if f.Face != nil {
		t.Errorf("札には顔を付けない")
	}
	f, _ = ParseFigure(`{"parts":[{"shape":"sphere","size":[1],"pos":[0,0,0]}],"face":{"part":5}}`)
	if f.Face != nil {
		t.Errorf("無い部品の顔は消す")
	}
	if _, err := ParseFigure(`{"parts":[]}`); !errors.Is(err, ErrAIOutput) {
		t.Errorf("部品0個は ErrAIOutput: %v", err)
	}
	many := `{"parts":[` + strings.TrimSuffix(strings.Repeat(`{"shape":"sphere","size":[0.1],"pos":[0,0,0]},`, 60), ",") + `]}`
	if f, _ := ParseFigure(many); len(f.Parts) != FigurePartsMax {
		t.Errorf("部品は %d 個まで: %d", FigurePartsMax, len(f.Parts))
	}
}
