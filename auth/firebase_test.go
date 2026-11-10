package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

const project = "test-project"

// テスト用の鍵で、本物と同じ形の ID トークンを作る。
func makeToken(t *testing.T, key *rsa.PrivateKey, kid string, c map[string]any) string {
	t.Helper()
	enc := func(v any) string {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return base64.RawURLEncoding.EncodeToString(b)
	}
	signing := enc(map[string]string{"alg": "RS256", "kid": kid, "typ": "JWT"}) + "." + enc(c)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func TestVerify(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	fetches := 0
	v := &Verifier{
		ProjectID: project,
		Now:       func() time.Time { return now },
		FetchCerts: func(context.Context) (map[string]*rsa.PublicKey, time.Duration, error) {
			fetches++
			return map[string]*rsa.PublicKey{"k1": &key.PublicKey}, time.Hour, nil
		},
	}
	good := func() map[string]any {
		return map[string]any{
			"iss": "https://securetoken.google.com/" + project, "aud": project, "sub": "uid-123",
			"iat": now.Add(-time.Minute).Unix(), "exp": now.Add(time.Hour).Unix(), "auth_time": now.Add(-time.Minute).Unix(),
		}
	}
	with := func(k string, val any) map[string]any { c := good(); c[k] = val; return c }

	uid, err := v.Verify(context.Background(), makeToken(t, key, "k1", good()))
	if err != nil || uid != "uid-123" {
		t.Fatalf("正しいトークン: uid=%q err=%v", uid, err)
	}

	bad := []struct {
		name  string
		token string
	}{
		{"期限切れ", makeToken(t, key, "k1", with("exp", now.Add(-time.Hour).Unix()))},
		{"未来の発行時刻", makeToken(t, key, "k1", with("iat", now.Add(time.Hour).Unix()))},
		{"別プロジェクト宛て", makeToken(t, key, "k1", with("aud", "other"))},
		{"発行者が違う", makeToken(t, key, "k1", with("iss", "https://evil.example"))},
		{"ユーザー ID が空", makeToken(t, key, "k1", with("sub", ""))},
		{"別の鍵で署名", makeToken(t, other, "k1", good())},
		{"知らない kid", makeToken(t, key, "zz", good())},
		{"形が JWT でない", "abc"},
		{"空", ""},
	}
	for _, c := range bad {
		t.Run(c.name, func(t *testing.T) {
			if _, err := v.Verify(context.Background(), c.token); !errors.Is(err, ErrInvalidToken) {
				t.Fatalf("want ErrInvalidToken, got %v", err)
			}
		})
	}

	// 改ざん：中身だけ書き換えると署名が合わない
	tok := makeToken(t, key, "k1", good())
	parts := splitToken(tok)
	forged := parts[0] + "." + base64.RawURLEncoding.EncodeToString([]byte(`{"iss":"https://securetoken.google.com/test-project","aud":"test-project","sub":"admin","exp":9999999999,"iat":0}`)) + "." + parts[2]
	if _, err := v.Verify(context.Background(), forged); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("改ざんしたトークンが通った: %v", err)
	}

	if fetches > 2 {
		t.Errorf("公開鍵はキャッシュして使うこと（取得 %d 回）", fetches)
	}
}

func splitToken(s string) [3]string {
	var out [3]string
	i := 0
	start := 0
	for j := 0; j < len(s); j++ {
		if s[j] == '.' {
			out[i] = s[start:j]
			i++
			start = j + 1
		}
	}
	out[2] = s[start:]
	return out
}

func TestMaxAge(t *testing.T) {
	if got := maxAge("public, max-age=19302, must-revalidate"); got != 19302*time.Second {
		t.Fatalf("got %v", got)
	}
	if got := maxAge(""); got != time.Hour {
		t.Fatalf("default: got %v", got)
	}
}

func TestKeyRefetchIsThrottled(t *testing.T) {
	const project = "test-project"
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	fetches, fail := 0, false
	v := &Verifier{
		ProjectID: project,
		Now:       func() time.Time { return now },
		FetchCerts: func(context.Context) (map[string]*rsa.PublicKey, time.Duration, error) {
			fetches++
			if fail {
				return nil, 0, errors.New("google down")
			}
			return map[string]*rsa.PublicKey{"k1": &key.PublicKey}, time.Hour, nil
		},
	}
	claims := func() map[string]any {
		return map[string]any{
			"iss": "https://securetoken.google.com/" + project, "aud": project, "sub": "uid-1",
			"iat": now.Add(-time.Minute).Unix(), "exp": now.Add(2 * time.Hour).Unix(), "auth_time": now.Add(-time.Minute).Unix(),
		}
	}
	if _, err := v.Verify(context.Background(), makeToken(t, key, "k1", claims())); err != nil {
		t.Fatal(err)
	}
	// でたらめな kid を100回送っても、取り直しは1分に1回まで
	for i := 0; i < 100; i++ {
		if _, err := v.Verify(context.Background(), makeToken(t, key, "evil", claims())); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("知らない kid: %v", err)
		}
	}
	if fetches > 1 {
		t.Fatalf("知らない kid で取り直しすぎ: %d 回", fetches)
	}
	now = now.Add(2 * time.Minute)
	v.Verify(context.Background(), makeToken(t, key, "evil", claims()))
	if fetches != 2 {
		t.Fatalf("1分たてば1回だけ取り直す: %d 回", fetches)
	}
	// 鍵の期限が切れて、しかも Google から取れないときも、前の鍵で続ける
	now = now.Add(2 * time.Hour)
	fail = true
	if uid, err := v.Verify(context.Background(), makeToken(t, key, "k1", claims())); err != nil || uid != "uid-1" {
		t.Fatalf("取得に失敗しても古い鍵で確認できる: %q %v", uid, err)
	}
}
