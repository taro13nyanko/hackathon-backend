package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"hackathon-backend/gemini"
)

// 出品の確認用の Client だけが seed と mediaResolution を送る（模型づくり・説明文と同じ既定の Client は送らない）。
func TestModerationClientSendsSeedAndResolution(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = nil
		json.Unmarshal(b, &got)
		io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"{}"}]}}]}`)
	}))
	defer srv.Close()
	tok := func(context.Context) (string, error) { return "tok", nil }
	img := []gemini.Image{{MimeType: "image/jpeg", Data: []byte("abc")}}
	send := func(c *gemini.Client) map[string]any {
		c.Endpoint, c.Token = srv.URL, tok
		if _, err := c.GenerateFromImages(context.Background(), "s", "p", img); err != nil {
			t.Fatal(err)
		}
		gc, _ := got["generationConfig"].(map[string]any)
		return gc
	}

	gc := send(moderationClient("p"))
	if gc["seed"] != float64(moderationSeed) {
		t.Errorf("出品の確認は seed を固定して送る: %v", gc)
	}
	if gc["mediaResolution"] != "MEDIA_RESOLUTION_HIGH" {
		t.Errorf("出品の確認は画像を細かく読ませる: %v", gc)
	}
	if gc["temperature"] != 0.5 {
		t.Errorf("temperature は変えない: %v", gc["temperature"])
	}
	if gc["responseSchema"] == nil {
		t.Errorf("出力の形（responseSchema）は今までどおり送る: %v", gc)
	}

	gc = send(&gemini.Client{Model: "m"})
	if _, ok := gc["seed"]; ok {
		t.Errorf("既定の Client は seed を送らない: %v", gc)
	}
	if _, ok := gc["mediaResolution"]; ok {
		t.Errorf("既定の Client は mediaResolution を送らない: %v", gc)
	}
}
