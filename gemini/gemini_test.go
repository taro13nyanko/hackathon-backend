package gemini

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGenerate(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(401)
			return
		}
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &got)
		io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"考え中","thought":true},{"text":"  説明文です。 "}]},"finishReason":"STOP"}]}`)
	}))
	defer srv.Close()
	c := &Client{Endpoint: srv.URL, Token: func(context.Context) (string, error) { return "tok", nil }}
	text, err := c.Generate(context.Background(), "指示", "入力")
	if err != nil || text != "説明文です。" {
		t.Fatalf("%q %v", text, err)
	}
	if !strings.Contains(mustJSON(got), "指示") || !strings.Contains(mustJSON(got), "入力") {
		t.Fatalf("指示と入力を送ること: %v", got)
	}

	c.Token = func(context.Context) (string, error) { return "bad", nil }
	if _, err := c.Generate(context.Background(), "s", "p"); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("エラーを返すこと: %v", err)
	}
}

func TestURL(t *testing.T) {
	c := &Client{Project: "p", Location: "global", Model: "m"}
	if got := c.url(); got != "https://aiplatform.googleapis.com/v1/projects/p/locations/global/publishers/google/models/m:generateContent" {
		t.Fatal(got)
	}
	c.Location = "us-central1"
	if !strings.HasPrefix(c.url(), "https://us-central1-aiplatform.googleapis.com/") {
		t.Fatal(c.url())
	}
}

func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }

func TestGenerateFromImages(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &got)
		io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"{\"parts\":[]}"}]}}]}`)
	}))
	defer srv.Close()
	c := &Client{Endpoint: srv.URL, Token: func(context.Context) (string, error) { return "tok", nil }}
	text, err := c.GenerateFromImages(context.Background(), "指示", "写真です", []Image{{MimeType: "image/png", Data: []byte("abc")}})
	if err != nil || text != `{"parts":[]}` {
		t.Fatalf("%q %v", text, err)
	}
	s := mustJSON(got)
	for _, want := range []string{`"mimeType":"image/png"`, `"data":"YWJj"`, `"responseMimeType":"application/json"`, "写真です"} {
		if !strings.Contains(s, want) {
			t.Errorf("%s を送ること: %s", want, s)
		}
	}
}

func TestImageConfig(t *testing.T) {
	c := &Client{ThinkingLevel: "medium", JSONSchema: map[string]any{"type": "OBJECT"}}
	s := mustJSON(c.imageConfig())
	for _, want := range []string{`"thinkingLevel":"medium"`, `"responseSchema":{"type":"OBJECT"}`, `"responseMimeType":"application/json"`} {
		if !strings.Contains(s, want) {
			t.Errorf("%s が無い: %s", want, s)
		}
	}
	if s := mustJSON((&Client{}).imageConfig()); strings.Contains(s, "thinkingConfig") || strings.Contains(s, "responseSchema") {
		t.Errorf("設定が無ければ付けない: %s", s)
	}
}
