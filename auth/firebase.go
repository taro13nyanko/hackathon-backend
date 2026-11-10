// Package auth は Firebase Authentication の ID トークンを確かめる。
//
// ID トークンは Google の秘密鍵で署名された JWT（RS256）。公開鍵（X.509 証明書）は
// 下の URL で公開されているので、それで署名を確かめ、中身（期限・発行者・宛先）を見る。
// 確かめ方は Firebase の公式ドキュメント「サードパーティの JWT ライブラリを使用して ID トークンを確認する」のとおり。
// 外部ライブラリを増やさないよう、Go の標準ライブラリだけで書いている。
package auth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// CertsURL は ID トークンの署名を確かめる公開鍵の置き場所。
const CertsURL = "https://www.googleapis.com/robot/v1/metadata/x509/securetoken@system.gserviceaccount.com"

// ErrInvalidToken はトークンが無効なとき（→ 401）。
var ErrInvalidToken = errors.New("invalid id token")

// Verifier は Firebase の ID トークンを確かめる。
type Verifier struct {
	ProjectID string
	// 以下はテストで差し替えるため。nil なら本物を使う。
	FetchCerts func(ctx context.Context) (map[string]*rsa.PublicKey, time.Duration, error)
	Now        func() time.Time

	mu        sync.Mutex
	keys      map[string]*rsa.PublicKey
	expires   time.Time
	lastFetch time.Time  // 最後に公開鍵を取りに行った時刻
	fetchMu   sync.Mutex // 取りに行くのは同時に1つだけ
}

// NewVerifier は projectID（Firebase のプロジェクト ID）用の Verifier を作る。
func NewVerifier(projectID string) *Verifier {
	return &Verifier{ProjectID: projectID}
}

type header struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
}

type claims struct {
	Iss      string `json:"iss"`
	Aud      string `json:"aud"`
	Sub      string `json:"sub"`
	Exp      int64  `json:"exp"`
	Iat      int64  `json:"iat"`
	AuthTime int64  `json:"auth_time"`
}

// Verify はトークンを確かめ、Firebase のユーザー ID（uid）を返す。
func (v *Verifier) Verify(ctx context.Context, token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", ErrInvalidToken
	}
	var h header
	if err := decodePart(parts[0], &h); err != nil || h.Alg != "RS256" || h.Kid == "" {
		return "", ErrInvalidToken
	}
	var c claims
	if err := decodePart(parts[1], &c); err != nil {
		return "", ErrInvalidToken
	}

	// 中身の確認（期限・発行者・宛先・ユーザー ID）
	now := v.now().Unix()
	const skew = 60 // 時計のずれを1分まで許す
	switch {
	case c.Exp <= now-skew,
		c.Iat > now+skew,
		c.AuthTime > now+skew,
		c.Aud != v.ProjectID,
		c.Iss != "https://securetoken.google.com/"+v.ProjectID,
		c.Sub == "" || len(c.Sub) > 128:
		return "", ErrInvalidToken
	}

	// 署名の確認
	key, err := v.key(ctx, h.Kid)
	if err != nil {
		return "", err
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", ErrInvalidToken
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, sum[:], sig); err != nil {
		return "", ErrInvalidToken
	}
	return c.Sub, nil
}

func (v *Verifier) now() time.Time {
	if v.Now != nil {
		return v.Now()
	}
	return time.Now()
}

// RefetchMin は、知らない kid が来たときに公開鍵を取り直す最短の間隔。
// でたらめな kid の偽トークンを連打されても、Google への取得は1分に1回までになる。
const RefetchMin = time.Minute

// key は kid に対応する公開鍵を返す。鍵はキャッシュし、期限が切れたら取り直す。
// 取得に失敗しても、前に取れた鍵があればそれを使い続ける。
func (v *Verifier) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	if k, fresh := v.cached(kid); k != nil && fresh {
		return k, nil
	}
	// 取りに行くのは同時に1つだけ（ほかの人は、取り終わるのを待ってから表を見直す）
	v.fetchMu.Lock()
	defer v.fetchMu.Unlock()
	k, fresh := v.cached(kid)
	if k != nil && fresh {
		return k, nil
	}
	v.mu.Lock()
	hasKeys, recent := v.keys != nil, !v.lastFetch.IsZero() && v.now().Sub(v.lastFetch) < RefetchMin
	v.mu.Unlock()
	if hasKeys && recent {
		if k != nil {
			return k, nil // 期限切れだが、取り直したばかりなので古い鍵で続ける
		}
		return nil, ErrInvalidToken // 知らない kid。直前に取り直しているので、もう取りに行かない
	}

	fetch := v.FetchCerts
	if fetch == nil {
		fetch = fetchCerts
	}
	keys, maxAge, err := fetch(ctx)
	v.mu.Lock()
	v.lastFetch = v.now()
	if err == nil {
		v.keys, v.expires = keys, v.now().Add(maxAge)
	}
	k2 := v.keys[kid]
	v.mu.Unlock()
	if err != nil {
		if k != nil {
			return k, nil
		}
		if !hasKeys {
			return nil, fmt.Errorf("公開鍵の取得: %w", err)
		}
		return nil, ErrInvalidToken
	}
	if k2 == nil {
		return nil, ErrInvalidToken
	}
	return k2, nil
}

// cached は表にある鍵と、表がまだ期限内かを返す。
func (v *Verifier) cached(kid string) (*rsa.PublicKey, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.keys[kid], v.keys != nil && !v.now().After(v.expires)
}

func decodePart(s string, v any) error {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// certsClient は公開鍵の取得用（Google が応答しないときに待ち続けないよう、5秒で打ち切る）。
var certsClient = &http.Client{Timeout: 5 * time.Second}

// fetchCerts は Google から公開鍵（kid → PEM 形式の証明書）を取ってくる。
// Cache-Control の max-age の間はキャッシュしてよい、と決められている。
func fetchCerts(ctx context.Context) (map[string]*rsa.PublicKey, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, CertsURL, nil)
	if err != nil {
		return nil, 0, err
	}
	res, err := certsClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("status %d", res.StatusCode)
	}
	var pems map[string]string
	if err := json.NewDecoder(res.Body).Decode(&pems); err != nil {
		return nil, 0, err
	}
	keys, err := ParseCerts(pems)
	if err != nil {
		return nil, 0, err
	}
	return keys, maxAge(res.Header.Get("Cache-Control")), nil
}

// ParseCerts は kid → PEM 証明書 の対応表を、kid → 公開鍵 に変える。
func ParseCerts(pems map[string]string) (map[string]*rsa.PublicKey, error) {
	keys := make(map[string]*rsa.PublicKey, len(pems))
	for kid, p := range pems {
		block, _ := pem.Decode([]byte(p))
		if block == nil {
			return nil, fmt.Errorf("kid %s: PEM ではない", kid)
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("kid %s: %w", kid, err)
		}
		pub, ok := cert.PublicKey.(*rsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("kid %s: RSA の鍵ではない", kid)
		}
		keys[kid] = pub
	}
	return keys, nil
}

// maxAge は "public, max-age=19302, must-revalidate" から 19302 秒を取り出す（無ければ1時間）。
func maxAge(cacheControl string) time.Duration {
	for _, part := range strings.Split(cacheControl, ",") {
		part = strings.TrimSpace(part)
		if v, ok := strings.CutPrefix(part, "max-age="); ok {
			var n int
			if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
				return time.Duration(n) * time.Second
			}
		}
	}
	return time.Hour
}
