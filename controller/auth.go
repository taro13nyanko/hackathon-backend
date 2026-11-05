package controller

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"

	"hackathon-backend/auth"
	"hackathon-backend/model"
	"hackathon-backend/usecase"
)

// TokenVerifier は ID トークンを確かめて Firebase の uid を返す（本物は auth.Verifier）。
type TokenVerifier interface {
	Verify(ctx context.Context, token string) (string, error)
}

// Auth はログインが必要な API の前に置く確認係。
type Auth struct {
	Verifier TokenVerifier
	Members  *usecase.MemberUsecase
}

// RequireLogin は Authorization: Bearer <ID トークン> を確かめ、uid を渡して h を呼ぶ。
// トークンが無い・無効なら 401 unauthorized。
func (a *Auth) RequireLogin(h func(w http.ResponseWriter, r *http.Request, uid string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || token == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		uid, err := a.Verifier.Verify(r.Context(), token)
		if errors.Is(err, auth.ErrInvalidToken) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if err != nil { // 公開鍵が取れないなど、こちら側の問題
			log.Printf("fail: verify token, %v", err)
			writeError(w, http.StatusInternalServerError, "internal")
			return
		}
		h(w, r, uid)
	}
}

// RequireMember は RequireLogin に加えて、プロフィール登録済みかを確かめる。
// 未登録なら 403 profile_required（画面はプロフィール登録へ進む）。
func (a *Auth) RequireMember(h func(w http.ResponseWriter, r *http.Request, me model.Member)) http.HandlerFunc {
	return a.RequireLogin(func(w http.ResponseWriter, r *http.Request, uid string) {
		me, err := a.Members.Me(uid)
		if errors.Is(err, model.ErrNotFound) {
			writeError(w, http.StatusForbidden, "profile_required")
			return
		}
		if err != nil {
			writeErr(w, err)
			return
		}
		h(w, r, me)
	})
}
