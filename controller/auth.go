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

// RequireAdmin は RequireMember に加えて、管理者（role が admin）かを確かめる。違えば 403 forbidden。
func (a *Auth) RequireAdmin(h func(w http.ResponseWriter, r *http.Request, me model.Member)) http.HandlerFunc {
	return a.RequireMember(func(w http.ResponseWriter, r *http.Request, me model.Member) {
		if !me.IsAdmin() {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		h(w, r, me)
	})
}

// OptionalLogin はログインしなくても使える API 用（商品の一覧・詳細）。
// Authorization ヘッダがあって有効で、プロフィール登録済みなら本人を渡す。
// それ以外（ヘッダ無し・トークンが無効・未登録・確かめられない）は、エラーにせず未ログイン（nil）として h を呼ぶ。
func (a *Auth) OptionalLogin(h func(w http.ResponseWriter, r *http.Request, me *model.Member)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h(w, r, a.optionalMember(r))
	}
}

func (a *Auth) optionalMember(r *http.Request) *model.Member {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || token == "" {
		return nil
	}
	uid, err := a.Verifier.Verify(r.Context(), token)
	if err != nil {
		if !errors.Is(err, auth.ErrInvalidToken) {
			log.Printf("fail: verify token (optional), %v", err)
		}
		return nil
	}
	me, err := a.Members.Me(uid)
	if err != nil {
		if !errors.Is(err, model.ErrNotFound) {
			log.Printf("fail: member (optional), %v", err)
		}
		return nil
	}
	return &me
}
