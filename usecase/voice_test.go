package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"hackathon-backend/model"
)

func TestParseVoiceSearch(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    VoiceSearchRes
		wantErr error
	}{
		{"そのまま", `{"transcript":"三千円以下の教科書","q":"教科書","min_price":0,"max_price":3000}`,
			VoiceSearchRes{"三千円以下の教科書", "教科書", 0, 3000}, nil},
		{"コードブロックで囲まれていても読む", "```json\n{\"transcript\":\"本\",\"q\":\"本\",\"min_price\":0,\"max_price\":0}\n```",
			VoiceSearchRes{"本", "本", 0, 0}, nil},
		{"min>max は入れ替える", `{"transcript":"t","q":"本","min_price":1300,"max_price":700}`, VoiceSearchRes{"t", "本", 700, 1300}, nil},
		{"負は 0、上限より上は上限", `{"transcript":"t","q":"","min_price":-500,"max_price":99999999}`, VoiceSearchRes{"t", "", 0, ItemPriceMax}, nil},
		{"小数は四捨五入", `{"transcript":"t","q":"","min_price":699.6,"max_price":1300.4}`, VoiceSearchRes{"t", "", 700, 1300}, nil},
		{"検索語の空白は1つにまとめて40字で切る", `{"transcript":"t","q":"  線形　代数  ` + strings.Repeat("あ", 50) + `","min_price":0,"max_price":0}`,
			VoiceSearchRes{"t", cutRunes("線形 代数 "+strings.Repeat("あ", 50), VoiceQueryMax), 0, 0}, nil},
		{"transcript は200字で切る", `{"transcript":"` + strings.Repeat("い", 250) + `","q":"","min_price":0,"max_price":0}`,
			VoiceSearchRes{strings.Repeat("い", VoiceTranscriptMax), "", 0, 0}, nil},
		{"JSON でない", `わかりません`, VoiceSearchRes{}, ErrAIOutput},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseVoiceSearch(c.raw)
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("err: got %v, want %v", err, c.wantErr)
			}
			if got != c.want {
				t.Errorf("got %+v, want %+v", got, c.want)
			}
			if len([]rune(got.Q)) > VoiceQueryMax {
				t.Errorf("q は %d 字まで: %q", VoiceQueryMax, got.Q)
			}
		})
	}
}

func TestVoiceSearch(t *testing.T) {
	me := model.Member{Id: "M1"}
	audio := []byte("OggS....")
	boom := errors.New("gemini down")
	ok := `{"transcript":"千円くらいの電卓","q":"電卓","min_price":700,"max_price":1300}`
	cases := []struct {
		name     string
		in       VoiceInput
		out      string
		genErr   error
		wantErr  error
		wantMime string
	}{
		{"webm", VoiceInput{audio, "audio/webm"}, ok, nil, nil, "audio/webm"},
		{"codecs の付け足し・大文字は除いて確かめる", VoiceInput{audio, " Audio/WebM;codecs=opus"}, ok, nil, nil, "audio/webm"},
		{"ogg", VoiceInput{audio, "audio/ogg"}, ok, nil, nil, "audio/ogg"},
		{"mp4", VoiceInput{audio, "audio/mp4"}, ok, nil, nil, "audio/mp4"},
		{"mpeg", VoiceInput{audio, "audio/mpeg"}, ok, nil, nil, "audio/mpeg"},
		{"wav", VoiceInput{audio, "audio/wav"}, ok, nil, nil, "audio/wav"},
		{"一覧に無い mime は 400", VoiceInput{audio, "video/mp4"}, ok, nil, ErrInvalidInput, ""},
		{"mime なしは 400", VoiceInput{audio, ""}, ok, nil, ErrInvalidInput, ""},
		{"音声なしは 400", VoiceInput{nil, "audio/webm"}, ok, nil, ErrInvalidInput, ""},
		{"1MB ちょうどは通る", VoiceInput{make([]byte, VoiceAudioMax), "audio/webm"}, ok, nil, nil, "audio/webm"},
		{"1MB を超えたら 400", VoiceInput{make([]byte, VoiceAudioMax+1), "audio/webm"}, ok, nil, ErrInvalidInput, ""},
		{"Gemini の失敗はそのまま返す", VoiceInput{audio, "audio/webm"}, "", boom, boom, "audio/webm"},
		{"読めない出力は ErrAIOutput", VoiceInput{audio, "audio/webm"}, "えっと", nil, ErrAIOutput, "audio/webm"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gen := &fakeJSONGen{out: c.out, err: c.genErr}
			uc := &VoiceUsecase{Gen: gen}
			res, err := uc.Search(context.Background(), me, c.in)
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("err: got %v, want %v", err, c.wantErr)
			}
			if c.wantMime == "" {
				if gen.calls != 0 {
					t.Fatalf("入力が決まりの外なら Gemini を呼ばない")
				}
				return
			}
			if len(gen.images) != 1 || gen.images[0].MimeType != c.wantMime || len(gen.images[0].Data) != len(c.in.Audio) || gen.system != voiceSystem {
				t.Fatalf("音声を inlineData として1つ、そろえた mime で渡す: %d", len(gen.images))
			}
			if c.wantErr == nil && (res.Q != "電卓" || res.MinPrice != 700 || res.MaxPrice != 1300 || res.Transcript != "千円くらいの電卓") {
				t.Errorf("got %+v", res)
			}
		})
	}
	if _, err := (&VoiceUsecase{Gen: &fakeJSONGen{out: ok}}).Search(context.Background(), model.Member{}, VoiceInput{audio, "audio/webm"}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("会員 ID が空なら 400: %v", err)
	}
}

func TestVoiceSearchLimit(t *testing.T) {
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	limit := NewLimiter(VoicePerMinute, time.Minute)
	limit.Now = func() time.Time { return now }
	gen := &fakeJSONGen{out: `{"transcript":"本","q":"本","min_price":0,"max_price":0}`}
	uc := &VoiceUsecase{Gen: gen, Limit: limit}
	in := VoiceInput{[]byte("x"), "audio/webm"}
	for i := 0; i < VoicePerMinute; i++ {
		if _, err := uc.Search(context.Background(), model.Member{Id: "M1"}, in); err != nil {
			t.Fatalf("%d 回目: %v", i+1, err)
		}
	}
	if _, err := uc.Search(context.Background(), model.Member{Id: "M1"}, in); !errors.Is(err, ErrTooMany) || gen.calls != VoicePerMinute {
		t.Fatalf("1分に %d 回まで: %v calls=%d", VoicePerMinute, err, gen.calls)
	}
	now = now.Add(time.Minute)
	if _, err := uc.Search(context.Background(), model.Member{Id: "M1"}, in); err != nil {
		t.Fatalf("1分たてばまた使える: %v", err)
	}
}
