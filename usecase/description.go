package usecase

import (
	"context"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"hackathon-backend/model"
)

// Gemini への入力の決まり。
const (
	DescNotesMax  = 500 // メモの最大文字数
	DescPerMinute = 5   // 1人1分あたりの回数（使いすぎ防止）
)

// Generator は文章を作るもの（本物は gemini.Client）。
type Generator interface {
	Generate(ctx context.Context, system, prompt string) (string, error)
}

// DescriptionReq は POST /items/description のリクエストボディ。
type DescriptionReq struct {
	Title string `json:"title"`
	Notes string `json:"notes"`
}

type DescriptionUsecase struct {
	Gen Generator
	Now func() time.Time

	mu   sync.Mutex
	used map[string][]time.Time // 人ごとの最近の呼び出し時刻
}

const descSystem = `あなたはフリマアプリの出品を手伝うアシスタントです。
商品名とメモから、買い手に伝わる商品説明文を日本語で書いてください。

守ること：
- メモに書かれた事実だけを使う。メモに無いこと（ブランド、型番、サイズ、傷や汚れの有無、使用期間、付属品、受け渡し方法など）は書かない・推測しない。
- 誇張しない（「美品」「最高」などはメモにある場合だけ）。
- 300字以内。です・ます調。見出しや記号の飾り、絵文字は使わない。短い段落か箇条書き（「・」）で読みやすく。
- 説明文だけを出力する（前置きや「以下が説明文です」は書かない）。`

// Draft は説明文の下書きを作る。出品者が直してから出品する前提。
func (uc *DescriptionUsecase) Draft(ctx context.Context, me model.Member, req DescriptionReq) (string, error) {
	title := strings.TrimSpace(req.Title)
	notes := strings.TrimSpace(req.Notes)
	if title == "" || utf8.RuneCountInString(title) > ItemTitleMax || utf8.RuneCountInString(notes) > DescNotesMax {
		return "", ErrInvalidInput
	}
	if !uc.allow(me.Id) {
		return "", ErrTooMany
	}
	if notes == "" {
		notes = "（メモなし）"
	}
	text, err := uc.Gen.Generate(ctx, descSystem, "商品名：「"+title+"」\nメモ：\n"+notes)
	if err != nil {
		return "", err
	}
	text = strings.TrimSpace(text)
	if r := []rune(text); len(r) > DescriptionCut {
		text = string(r[:DescriptionCut])
	}
	return text, nil
}

// DescriptionCut は返す説明文の上限（出品の説明は1000文字までなので、それより短く切る）。
const DescriptionCut = 600

// allow は1人1分あたり DescPerMinute 回までにする（Cloud Run のインスタンスごと。目安）。
func (uc *DescriptionUsecase) allow(id string) bool {
	now := time.Now()
	if uc.Now != nil {
		now = uc.Now()
	}
	uc.mu.Lock()
	defer uc.mu.Unlock()
	if uc.used == nil {
		uc.used = map[string][]time.Time{}
	}
	recent := uc.used[id][:0]
	for _, t := range uc.used[id] {
		if now.Sub(t) < time.Minute {
			recent = append(recent, t)
		}
	}
	if len(recent) >= DescPerMinute {
		uc.used[id] = recent
		return false
	}
	uc.used[id] = append(recent, now)
	return true
}
