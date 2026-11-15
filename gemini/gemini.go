// Package gemini は Vertex AI の Gemini を呼ぶ。
//
// Go の SDK（google.golang.org/genai）は依存が多いので、REST API（generateContent）を標準ライブラリだけで呼ぶ。
// 認証は Cloud Run のサービスアカウント：メタデータサーバーからアクセストークンをもらう（API キーは置かない）。
// このため、Cloud Run のサービスアカウントに「Vertex AI ユーザー」ロールが必要。
package gemini

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ErrUnavailable は Gemini を使えない（ローカルでトークンが取れない等）とき。
var ErrUnavailable = errors.New("gemini unavailable")

type Client struct {
	Project  string // GCP プロジェクト ID
	Location string // 例: global
	Model    string // 例: gemini-flash-latest

	// 画像から JSON を作るとき（GenerateFromImage）だけ使う設定。空なら付けない
	ThinkingLevel string         // 考える深さ: low / medium / high（対応モデルのみ）
	JSONSchema    map[string]any // 出力の形（responseSchema）。形を外れた出力を減らす

	// 生成設定の上書き（今は出品の確認だけで使う）。設定したときだけ generationConfig に入れ、未設定なら今までと同じ内容を送る。
	// temperature は変えない（Gemini 3 系は下げないよう勧められているため）
	Seed            *int   // 乱数の種。固定すると、同じ入力で結果がそろいやすくなる
	MediaResolution string // 画像を読む細かさ: MEDIA_RESOLUTION_LOW / MEDIUM / HIGH。空なら付けない（モデルの既定）
	// noMediaRes は mediaResolution を付けて 400 が返ったあと true になる。以後はこの Client では付けない
	noMediaRes atomic.Bool

	HTTP     *http.Client
	Endpoint string                                    // テスト用。空なら Vertex AI
	Token    func(ctx context.Context) (string, error) // テスト用。nil ならメタデータサーバー

	mu      sync.Mutex
	token   string
	expires time.Time
}

// Generate は system（指示）と prompt（入力）を渡して、生成された文章を返す。
func (c *Client) Generate(ctx context.Context, system, prompt string) (string, error) {
	return c.generate(ctx, system, []map[string]any{{"text": prompt}},
		c.withOverrides(map[string]any{"temperature": 0.4, "maxOutputTokens": 2048}))
}

// Image は Gemini に渡す画像1枚。
type Image struct {
	MimeType string
	Data     []byte
}

// GenerateFromImages は画像（1枚以上）と prompt を渡して、JSON の文字列を返させる（responseMimeType: application/json）。
func (c *Client) GenerateFromImages(ctx context.Context, system, prompt string, images []Image) (string, error) {
	parts := make([]map[string]any, 0, len(images)+1)
	for _, img := range images {
		parts = append(parts, map[string]any{"inlineData": map[string]string{"mimeType": img.MimeType, "data": base64.StdEncoding.EncodeToString(img.Data)}})
	}
	parts = append(parts, map[string]any{"text": prompt})
	text, err := c.generate(ctx, system, parts, c.imageConfig())
	// モデルが mediaResolution を受け付けず 400 を返したときは、外して1回だけ送り直す。
	// こうしないと、出品の確認がすべて失敗（review）になる。一度 400 になったら、以後は付けない
	var se *statusError
	if err != nil && errors.As(err, &se) && se.code == http.StatusBadRequest && c.MediaResolution != "" && !c.noMediaRes.Load() {
		c.noMediaRes.Store(true)
		log.Printf("gemini: mediaResolution=%s を受け付けなかったので外して送り直す: %v", c.MediaResolution, err)
		return c.generate(ctx, system, parts, c.imageConfig())
	}
	return text, err
}

func (c *Client) imageConfig() map[string]any {
	config := map[string]any{"temperature": 0.5, "maxOutputTokens": 16384, "responseMimeType": "application/json"}
	if c.ThinkingLevel != "" {
		config["thinkingConfig"] = map[string]any{"thinkingLevel": c.ThinkingLevel}
	}
	if c.JSONSchema != nil {
		config["responseSchema"] = c.JSONSchema
	}
	return c.withOverrides(config)
}

// withOverrides は Seed・MediaResolution が設定されているときだけ config に足す。
func (c *Client) withOverrides(config map[string]any) map[string]any {
	if c.Seed != nil {
		config["seed"] = *c.Seed
	}
	if c.MediaResolution != "" && !c.noMediaRes.Load() {
		config["mediaResolution"] = c.MediaResolution
	}
	return config
}

func (c *Client) generate(ctx context.Context, system string, parts []map[string]any, config map[string]any) (string, error) {
	token, err := c.accessToken(ctx)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	body, _ := json.Marshal(map[string]any{
		"systemInstruction": map[string]any{"parts": []map[string]string{{"text": system}}},
		"contents":          []map[string]any{{"role": "user", "parts": parts}},
		"generationConfig":  config,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url(), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.httpClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("gemini: %w", err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode != http.StatusOK {
		return "", &statusError{code: res.StatusCode, body: truncate(string(raw), 300)}
	}
	var out struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text    string `json:"text"`
					Thought bool   `json:"thought"`
				} `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("gemini: decode: %w", err)
	}
	if len(out.Candidates) == 0 {
		return "", fmt.Errorf("gemini: no candidates: %s", truncate(string(raw), 300))
	}
	var sb strings.Builder
	for _, p := range out.Candidates[0].Content.Parts {
		if !p.Thought {
			sb.WriteString(p.Text)
		}
	}
	text := strings.TrimSpace(sb.String())
	if text == "" {
		return "", fmt.Errorf("gemini: empty text (finishReason=%s)", out.Candidates[0].FinishReason)
	}
	return text, nil
}

// statusError は Gemini が 200 以外を返したとき。code で 400 かどうかを見分ける。
type statusError struct {
	code int
	body string
}

func (e *statusError) Error() string { return fmt.Sprintf("gemini: status %d: %s", e.code, e.body) }

func (c *Client) url() string {
	if c.Endpoint != "" {
		return c.Endpoint
	}
	host := "aiplatform.googleapis.com"
	if c.Location != "global" {
		host = c.Location + "-aiplatform.googleapis.com"
	}
	return fmt.Sprintf("https://%s/v1/projects/%s/locations/%s/publishers/google/models/%s:generateContent",
		host, c.Project, c.Location, c.Model)
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 240 * time.Second} // 画像から模型を作るとき（最上位モデル）は1〜2分かかる。Cloud Run の上限 300 秒より短く
}

// accessToken はトークンをキャッシュして、切れる1分前に取り直す。
func (c *Client) accessToken(ctx context.Context) (string, error) {
	if c.Token != nil {
		return c.Token(ctx)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Now().Before(c.expires.Add(-time.Minute)) {
		return c.token, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Metadata-Flavor", "Google")
	res, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("metadata token: status %d", res.StatusCode)
	}
	var t struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(res.Body).Decode(&t); err != nil {
		return "", err
	}
	c.token, c.expires = t.AccessToken, time.Now().Add(time.Duration(t.ExpiresIn)*time.Second)
	return c.token, nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
