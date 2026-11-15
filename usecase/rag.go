package usecase

import (
	"math"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// 商品への質問（自前の RAG）の「検索（Retrieval）」の部分。外の検索エンジンや埋め込みは使わず、Go の標準ライブラリだけで書く。
//
//  1. 断片（Chunk）を Tokenize で語に分ける：日本語は文字バイグラム（2文字ずつずらして切る）、英数字は単語。
//     日本語は単語の区切りが無いので、形態素解析の辞書を持たずに部分一致に強い文字バイグラムを使う。
//  2. 質問との近さを BM25 で点数にする（珍しい語ほど重く、長い断片ほど1語の重みを下げる）。
//  3. SelectChunks で点数の高い順に選ぶ（点数0は使わない。必ず入れる断片は先頭に入れる）。
//
// どれも純粋な関数（DB や Gemini を呼ばない）なので、rag_test.go で値を固定してテストしている。

// 断片の種類（POST /items/{id}/ask の sources[].kind）。
const (
	ChunkItem       = "item"        // この商品（商品名・価格・状態・説明）
	ChunkSellerItem = "seller_item" // 同じ出品者の他の商品
	ChunkRule       = "rule"        // アプリの決まり（rules.go）
	ChunkFigure     = "figure"      // 保存済みの模型の説明
)

// Chunk は検索の対象になる文書の断片。
type Chunk struct {
	Kind  string
	Title string
	Text  string
}

// BM25 のパラメータ（よく使われる値）。
const (
	bm25K1 = 1.2  // 同じ語が何回も出たときの効き方（大きいほど回数が効く）
	bm25B  = 0.75 // 断片の長さで割り引く強さ（0 なら割り引かない）
)

// normalizeRune は全角の英数字・記号を半角に、全角スペースを半角に、英字を小文字にする。
func normalizeRune(r rune) rune {
	switch {
	case r >= '！' && r <= '～': // U+FF01〜U+FF5E は半角 ASCII と同じ並び
		r -= 0xFEE0
	case r == '　':
		r = ' '
	}
	return unicode.ToLower(r)
}

// isWordRune は英数字（単語として切る文字）か。ラテン文字（é など）も含める。
func isWordRune(r rune) bool {
	return unicode.Is(unicode.Latin, r) || unicode.IsDigit(r)
}

// Tokenize は文を検索用の語に分ける。
//   - 英数字の並び → 1語（小文字。全角も半角にそろえる）。例：「iPhone15」→「iphone15」
//   - それ以外の文字（ひらがな・カタカナ・漢字・長音など）の並び → 文字バイグラム。1文字だけならその1文字。
//     例：「線形代数」→「線形」「形代」「代数」
//   - 記号・空白・句読点は区切りとして捨てる。
func Tokenize(s string) []string {
	tokens := []string{}
	var word []rune // 英数字の並び
	var text []rune // 日本語などの並び
	flushWord := func() {
		if len(word) > 0 {
			tokens = append(tokens, string(word))
			word = word[:0]
		}
	}
	flushText := func() {
		switch {
		case len(text) == 1:
			tokens = append(tokens, string(text))
		case len(text) > 1:
			for i := 0; i+1 < len(text); i++ {
				tokens = append(tokens, string(text[i:i+2]))
			}
		}
		text = text[:0]
	}
	for _, r := range s {
		r = normalizeRune(r)
		switch {
		case isWordRune(r):
			flushText()
			word = append(word, r)
		case unicode.IsLetter(r): // 長音「ー」や「々」も文字に入る
			flushWord()
			text = append(text, r)
		default:
			flushWord()
			flushText()
		}
	}
	flushWord()
	flushText()
	return tokens
}

// BM25 は、語に分けた断片 docs それぞれの、語に分けた質問 query との近さを返す（docs と同じ並び）。
// 質問の語は重複を除いて数える（同じ語を繰り返しても点数は上がらない）。
//
//	score(d) = Σ_語 idf(語) × tf×(k1+1) / (tf + k1×(1 − b + b×|d|/平均の長さ))
//	idf(語) = ln(1 + (N − df + 0.5) / (df + 0.5))   （N＝断片の数、df＝その語を含む断片の数。いつも正）
func BM25(docs [][]string, query []string) []float64 {
	scores := make([]float64, len(docs))
	if len(docs) == 0 || len(query) == 0 {
		return scores
	}
	tfs := make([]map[string]int, len(docs))
	df := map[string]int{}
	total := 0
	for i, d := range docs {
		tfs[i] = map[string]int{}
		for _, t := range d {
			tfs[i][t]++
		}
		for t := range tfs[i] {
			df[t]++
		}
		total += len(d)
	}
	avg := float64(total) / float64(len(docs))
	if avg == 0 {
		return scores
	}
	n := float64(len(docs))
	seen := map[string]bool{}
	for _, q := range query {
		if seen[q] || df[q] == 0 {
			continue
		}
		seen[q] = true
		idf := math.Log(1 + (n-float64(df[q])+0.5)/(float64(df[q])+0.5))
		for i := range docs {
			tf := float64(tfs[i][q])
			if tf == 0 {
				continue
			}
			norm := 1 - bm25B + bm25B*float64(len(docs[i]))/avg
			scores[i] += idf * tf * (bm25K1 + 1) / (tf + bm25K1*norm)
		}
	}
	return scores
}

// SelectChunks は質問 question に近い断片を k 件まで選び、chunks の番号（0 始まり）を返す。
//   - 点数が0の断片（質問と同じ語が1つも無い）は使わない。
//   - must（0 以上なら）は点数に関係なく必ず入れ、先頭に置く（この商品の説明の先頭）。
//   - 残りは点数の高い順（同点なら chunks の前のほう）。
func SelectChunks(chunks []Chunk, question string, k, must int) []int {
	if k <= 0 || len(chunks) == 0 {
		return []int{}
	}
	docs := make([][]string, len(chunks))
	for i, c := range chunks {
		docs[i] = Tokenize(c.Title + " " + c.Text)
	}
	scores := BM25(docs, Tokenize(question))
	order := make([]int, 0, len(chunks))
	for i := range chunks {
		if scores[i] > 0 && i != must {
			order = append(order, i)
		}
	}
	sort.SliceStable(order, func(a, b int) bool { return scores[order[a]] > scores[order[b]] })
	picked := []int{}
	if must >= 0 && must < len(chunks) {
		picked = append(picked, must)
	}
	for _, i := range order {
		if len(picked) >= k {
			break
		}
		picked = append(picked, i)
	}
	return picked
}

// SplitSentences は説明文を文ごとの断片に分ける（。！？!? と改行で切る）。
// 短い文は max 文字までつなげ、max を超える1文は max 文字ずつに切る（断片の長さをそろえて BM25 の点数を比べやすくする）。
func SplitSentences(s string, max int) []string {
	var sentences []string
	var cur []rune
	cut := func() {
		if t := strings.TrimSpace(string(cur)); t != "" {
			sentences = append(sentences, t)
		}
		cur = cur[:0]
	}
	for _, r := range s {
		if r == '\n' || r == '\r' {
			cut()
			continue
		}
		cur = append(cur, r)
		if strings.ContainsRune("。！？!?", r) {
			cut()
		}
	}
	cut()

	out := []string{}
	var buf string
	for _, sen := range sentences {
		for utf8.RuneCountInString(sen) > max { // 長すぎる1文は切る
			if buf != "" {
				out, buf = append(out, buf), ""
			}
			r := []rune(sen)
			out, sen = append(out, string(r[:max])), string(r[max:])
		}
		if buf != "" && utf8.RuneCountInString(buf)+utf8.RuneCountInString(sen) > max {
			out, buf = append(out, buf), ""
		}
		buf += sen
	}
	if buf != "" {
		out = append(out, buf)
	}
	return out
}
