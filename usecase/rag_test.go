package usecase

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestTokenize(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"日本語は文字バイグラム", "線形代数", []string{"線形", "形代", "代数"}},
		{"1文字だけならその1文字", "本", []string{"本"}},
		{"英数字は単語・小文字", "iPhone15 ケース", []string{"iphone15", "ケー", "ース"}},
		{"全角の英数字・全角スペースは半角にそろえる", "ＡＢＣ１２３　本", []string{"abc123", "本"}},
		{"数字と日本語の境目で切る", "3000円以下", []string{"3000", "円以", "以下"}},
		{"記号・句読点は区切り", "受け渡し、駒場。", []string{"受け", "け渡", "渡し", "駒場"}},
		{"長音は文字に入る", "ノート", []string{"ノー", "ート"}},
		{"空", "", []string{}},
		{"記号だけ", "、。!?（）", []string{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Tokenize(c.in); !reflect.DeepEqual(got, c.want) {
				t.Errorf("Tokenize(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestBM25(t *testing.T) {
	docs := [][]string{{"a", "b"}, {"c", "d"}, {"a", "a", "c"}}
	s := BM25(docs, []string{"a"})
	// 手で計算した値：idf = ln(1+(3-2+0.5)/(2+0.5))、平均の長さ 7/3
	idf := math.Log(1 + 1.5/2.5)
	want0 := idf * 1 * 2.2 / (1 + 1.2*(0.25+0.75*2/(7.0/3)))
	want2 := idf * 2 * 2.2 / (2 + 1.2*(0.25+0.75*3/(7.0/3)))
	if math.Abs(s[0]-want0) > 1e-9 || s[1] != 0 || math.Abs(s[2]-want2) > 1e-9 {
		t.Fatalf("BM25 = %v, want [%v 0 %v]", s, want0, want2)
	}
	if !(s[2] > s[0]) {
		t.Errorf("同じ語が多い断片ほど点数が高い: %v", s)
	}

	// 珍しい語ほど重い
	docs = [][]string{{"x", "y"}, {"x", "z"}, {"x", "w"}}
	if common, rare := BM25(docs, []string{"x"})[0], BM25(docs, []string{"y"})[0]; !(rare > common && common > 0) {
		t.Errorf("珍しい語ほど重い（idf はいつも正）: x=%v y=%v", common, rare)
	}
	// 質問の語の重複は数えない
	if a, b := BM25(docs, []string{"y"}), BM25(docs, []string{"y", "y", "y"}); !reflect.DeepEqual(a, b) {
		t.Errorf("質問の語の重複で点数を上げない: %v %v", a, b)
	}
	// 当たらない語・空の質問・空の断片は 0
	for _, s := range [][]float64{BM25(docs, []string{"nothing"}), BM25(docs, nil), BM25([][]string{{}, {}}, []string{"x"})} {
		for _, v := range s {
			if v != 0 {
				t.Errorf("0 のはず: %v", s)
			}
		}
	}
	if len(BM25(nil, []string{"x"})) != 0 {
		t.Errorf("断片が無ければ空")
	}
}

func TestSelectChunks(t *testing.T) {
	chunks := []Chunk{
		{ChunkItem, "この商品の説明", "商品名：線形代数の教科書。価格：1200円。"},
		{ChunkRule, "支払い", "このアプリでは支払いは発生しません。"},
		{ChunkRule, "受け渡し", "受け渡しの方法は、出品者と購入者がメッセージで相談して決めます。"},
		{ChunkRule, "メッセージ", "出品者にメッセージを送れます。"},
		{ChunkSellerItem, "微積分の教科書", "商品名：微積分の教科書。"},
	}
	cases := []struct {
		name     string
		question string
		k, must  int
		want     []int
	}{
		{"必ず入れる断片は先頭、残りは点数順", "受け渡しの方法", 5, 0, []int{0, 2}},
		{"点数0の断片は使わない", "支払い", 5, 0, []int{0, 1}},
		{"k 件まで", "メッセージで受け渡し", 2, 0, []int{0, 2}},
		{"何も当たらなければ必ず入れる断片だけ", "zzz", 5, 0, []int{0}},
		{"must なし（-1）は点数順（短くて2回出る断片が上）", "教科書", 5, -1, []int{4, 0}},
		{"k が0なら空", "教科書", 0, 0, []int{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := SelectChunks(chunks, c.question, c.k, c.must); !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
	if got := SelectChunks(nil, "教科書", 5, 0); len(got) != 0 {
		t.Errorf("断片が無ければ空: %v", got)
	}
}

func TestSplitSentences(t *testing.T) {
	cases := []struct {
		name string
		in   string
		max  int
		want []string
	}{
		{"短い文はつなげる", "一文目です。二文目です！\n三", 100, []string{"一文目です。二文目です！三"}},
		{"max を超えたら分ける", "一文目です。二文目です！\n三", 6, []string{"一文目です。", "二文目です！", "三"}},
		{"長すぎる1文は max 文字ずつ", strings.Repeat("あ", 250), 120, []string{strings.Repeat("あ", 120), strings.Repeat("あ", 120), strings.Repeat("あ", 10)}},
		{"空白だけの行は捨てる", "  \n\n a。 ", 100, []string{"a。"}},
		{"空", "", 100, []string{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := SplitSentences(c.in, c.max); !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
