// Package gemini は Vertex AI の Gemini を呼ぶ。
//
// Go の SDK（google.golang.org/genai）は依存が多いので、REST API（generateContent）を標準ライブラリだけで呼ぶ。
// 認証は Cloud Run のサービスアカウント：メタデータサーバーからアクセストークンをもらう（API キーは置かない）。
// このため、Cloud Run のサービスアカウントに「Vertex AI ユーザー」ロールが必要。
package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ErrUnavailable は Gemini を使えない（ローカルでトークンが取れない等）とき。
var ErrUnavailable = errors.New("gemini unavailable")

type Client struct {
	Project  string // GCP プロジェクト ID
	Location string // 例: global
	Model    string // 例: gemini-flash-latest

	HTTP     *http.Client
	Endpoint string                                    // テスト用。空なら Vertex AI
	Token    func(ctx context.Context) (string, error) // テスト用。nil ならメタデータサーバー

	mu      sync.Mutex
	token   string
	expires time.Time
}

// Generate は system（指示）と prompt（入力）を渡して、生成された文章を返す。
func (c *Client) Generate(ctx context.Context, system, prompt string) (string, error) {
	token, err := c.accessToken(ctx)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	body, _ := json.Marshal(map[string]any{
		"systemInstruction": map[string]any{"parts": []map[string]string{{"text": system}}},
		"contents":          []map[string]any{{"role": "user", "parts": []map[string]string{{"text": prompt}}}},
		"generationConfig":  map[string]any{"temperature": 0.4, "maxOutputTokens": 2048},
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
		return "", fmt.Errorf("gemini: status %d: %s", res.StatusCode, truncate(string(raw), 300))
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
	return &http.Client{Timeout: 30 * time.Second}
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
