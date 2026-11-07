package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"hackathon-backend/model"
)

// 写真からデフォルメ3D模型の設計図（部品の JSON）を作る。
// 形は 試作/figure3d_prototype_v2.html の描き方に合わせている（画面はこの JSON をそのまま three.js で組み立てる）。

const (
	FigureImageMax  = 4 << 20 // 画像は 4MB まで
	FigurePartsMax  = 40      // 部品は 40 個まで（多すぎると重い）
	FigurePerMinute = 3       // 1人1分あたりの回数
	FigureNameMax   = 20
	FigureLabelMax  = 12 // 札の文字数
)

// ImageGenerator は画像から JSON を作るもの（本物は gemini.Client）。
type ImageGenerator interface {
	GenerateFromImage(ctx context.Context, system, prompt, mimeType string, image []byte) (string, error)
}

// FigurePart は部品1つ。
type FigurePart struct {
	Shape   string    `json:"shape"`
	Size    []float64 `json:"size"`
	Pos     []float64 `json:"pos"`
	Rot     []float64 `json:"rot,omitempty"`
	Scale   []float64 `json:"scale,omitempty"`
	Color   string    `json:"color,omitempty"`
	Round   *float64  `json:"round,omitempty"`
	Arc     *float64  `json:"arc,omitempty"`
	Glow    bool      `json:"glow,omitempty"`
	Outline *bool     `json:"outline,omitempty"`
	Text    string    `json:"text,omitempty"`
	Ink     string    `json:"ink,omitempty"`
	Bg      string    `json:"bg,omitempty"`
}

// FigureFace は顔。
type FigureFace struct {
	Part  int      `json:"part"`
	Style string   `json:"style"`
	Y     *float64 `json:"y,omitempty"`
	Scale *float64 `json:"scale,omitempty"`
}

// Figure は模型の設計図。
type Figure struct {
	Name    string       `json:"name"`
	Caption string       `json:"caption"`
	Parts   []FigurePart `json:"parts"`
	Face    *FigureFace  `json:"face,omitempty"`
}

type FigureUsecase struct {
	Gen ImageGenerator
	Now func() time.Time

	mu   sync.Mutex
	used map[string][]time.Time
}

const figureSystem = `あなたは、フリマアプリの商品写真から「デフォルメされたかわいい3D模型」の設計図を作るデザイナーです。
写真に写っている主な物を1つ選び、単純な立体（部品）の組み合わせで、ちびキャラ風に作ってください。

出力は次の形の JSON だけ（説明文は書かない）：
{"name": 物の名前(20字以内), "caption": 愛称つきの一言(30字以内、例「夜ふかしの相棒、ランプさん」),
 "parts": [部品...], "face": {"part": 顔を付ける部品の番号, "style": "smile"|"wink"|"sleepy"|"open", "y": 顔の高さ}}

部品の形（shape）と size：
- "box" [幅, 高さ, 奥行き]（角は丸い。"round": 0〜0.5 で丸さ）
- "cylinder" / "cone" [上の半径, 下の半径, 高さ]
- "sphere" [半径]、"dome" [半径]（半球。足など床に置くもの）
- "capsule" [半径, 胴の長さ]（腕・取っ手など）
- "torus" [半径, 太さ]（"arc": 0〜360 で一部だけの輪）
- "label" [幅, 高さ]（文字の札。"text": 12字以内, "ink": 文字色, "bg": 札の色（省略で透明））
共通："pos": [x, y, z]（y が上、z が手前、床は y=0）、"rot": [x, y, z]（度）、"scale": [x, y, z]、
"color": "#rrggbb"、"glow": true（光る部分）、"outline": false（細い線や小さい飾りには輪郭線を付けない）。

作り方のコツ：
- 全体の高さは 2 前後、横幅は 2 以内。物の真ん中を x=0, z=0 に置き、正面（顔の面）を z の手前に向ける。
- 部品は 12〜30 個。大きな形を先に置き、ふち・線・ボタン・ロゴの札・小物など細かい部品で写真らしさを出す。
- 色は写真の色を少し明るく、やさしくする。
- 顔は大きくて平らな面（正面の板・かさ等）に付ける。face.part はその部品の番号（0 から数える）。
- 小さな手足（capsule・dome）を付けるとかわいくなる。
- 人の顔・個人名・住所など個人が分かる文字は札に書かない。`

// ErrAIOutput は Gemini の出力が設計図として読めないとき（画面は「もう一度」を出す）。
var ErrAIOutput = errors.New("ai output invalid")

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

var figureShapes = map[string]int{ // 形 → size の数
	"box": 3, "cylinder": 3, "cone": 3, "sphere": 1, "dome": 1, "capsule": 2, "torus": 2, "label": 2,
}

var faceStyles = map[string]bool{"smile": true, "wink": true, "sleepy": true, "open": true}

// FromImage は写真から模型の設計図を作る。name は出品者が付けた商品名（ヒント。空でもよい）。
func (uc *FigureUsecase) FromImage(ctx context.Context, me model.Member, image []byte, name string) (Figure, error) {
	name = strings.TrimSpace(name)
	if len(image) == 0 || len(image) > FigureImageMax || utf8.RuneCountInString(name) > ItemTitleMax {
		return Figure{}, ErrInvalidInput
	}
	mime := http.DetectContentType(image)
	switch mime {
	case "image/jpeg", "image/png", "image/webp":
	default:
		return Figure{}, ErrInvalidInput
	}
	if !uc.allow(me.Id) {
		return Figure{}, ErrTooMany
	}
	prompt := "この写真の物を模型にしてください。"
	if name != "" {
		prompt += "商品名は「" + name + "」です。"
	}
	raw, err := uc.Gen.GenerateFromImage(ctx, figureSystem, prompt, mime, image)
	if err != nil {
		return Figure{}, err
	}
	return ParseFigure(raw)
}

// ParseFigure は Gemini の出力を読み、範囲外の値を直す。使える部品が1つも無ければ ErrAIOutput。
func ParseFigure(raw string) (Figure, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	var f Figure
	if err := json.Unmarshal([]byte(raw), &f); err != nil {
		return Figure{}, ErrAIOutput
	}
	f.Name = cutRunes(strings.TrimSpace(f.Name), FigureNameMax)
	f.Caption = cutRunes(strings.TrimSpace(f.Caption), 40)

	// 部品を1つずつ確かめる。だめな部品は捨て、顔の番号を付け直す
	newIndex := map[int]int{}
	parts := make([]FigurePart, 0, len(f.Parts))
	for i, p := range f.Parts {
		if len(parts) >= FigurePartsMax {
			break
		}
		if q, ok := cleanPart(p); ok {
			newIndex[i] = len(parts)
			parts = append(parts, q)
		}
	}
	if len(parts) == 0 {
		return Figure{}, ErrAIOutput
	}
	f.Parts = parts
	if f.Face != nil {
		j, ok := newIndex[f.Face.Part]
		if !ok || parts[j].Shape == "label" {
			f.Face = nil
		} else {
			f.Face.Part = j
			if !faceStyles[f.Face.Style] {
				f.Face.Style = "smile"
			}
			if f.Face.Y != nil {
				*f.Face.Y = clamp(*f.Face.Y, -1, 5)
			}
			if f.Face.Scale != nil {
				*f.Face.Scale = clamp(*f.Face.Scale, 0.3, 2)
			}
		}
	}
	return f, nil
}

func cleanPart(p FigurePart) (FigurePart, bool) {
	n, ok := figureShapes[p.Shape]
	if !ok || len(p.Size) < n || len(p.Pos) != 3 {
		return p, false
	}
	p.Size = p.Size[:n]
	for i := range p.Size {
		if !finite(p.Size[i]) {
			return p, false
		}
		p.Size[i] = clamp(p.Size[i], 0.005, 4)
	}
	if p.Shape == "cylinder" || p.Shape == "cone" { // 半径は 0 でもよい（とがった円すい）
		p.Size[0] = clamp(p.Size[0], 0, 4)
	}
	if !vec3(p.Pos, -5, 5) || (p.Rot != nil && !vec3(p.Rot, -360, 360)) || (p.Scale != nil && !vec3(p.Scale, 0.05, 5)) {
		return p, false
	}
	if p.Shape == "label" {
		p.Text = cutRunes(strings.TrimSpace(p.Text), FigureLabelMax)
		if p.Text == "" {
			return p, false
		}
		p.Ink = colorOr(p.Ink, "#222222")
		if p.Bg != "" {
			p.Bg = colorOr(p.Bg, "")
		}
		p.Color, p.Round, p.Arc, p.Glow, p.Outline = "", nil, nil, false, nil
		return p, true
	}
	p.Text, p.Ink, p.Bg = "", "", ""
	p.Color = colorOr(p.Color, "#cccccc")
	if p.Round != nil {
		*p.Round = clamp(*p.Round, 0, 0.5)
	}
	if p.Arc != nil {
		*p.Arc = clamp(*p.Arc, 10, 360)
	}
	return p, true
}

// vec3 は3つの有限な数かを確かめ、範囲に収める。
func vec3(v []float64, lo, hi float64) bool {
	if len(v) != 3 {
		return false
	}
	for i := range v {
		if !finite(v[i]) {
			return false
		}
		v[i] = clamp(v[i], lo, hi)
	}
	return true
}

func finite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }

func clamp(x, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, x)) }

func colorOr(c, def string) string {
	if hexColor.MatchString(c) {
		return strings.ToLower(c)
	}
	return def
}

func cutRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

func (uc *FigureUsecase) allow(id string) bool {
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
	if len(recent) >= FigurePerMinute {
		uc.used[id] = recent
		return false
	}
	uc.used[id] = append(recent, now)
	return true
}
