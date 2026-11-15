package controller

import (
	"encoding/json"
	"log"
	"net/http"

	"hackathon-backend/model"
	"hackathon-backend/usecase"
)

// AdminController は管理者の画面（どれもログイン＋表示名の登録＋管理者ロールが必要。違えば 403 forbidden）。
//
//	GET /admin/items?status=review|hidden    要確認（既定）か非公開の商品（新しい順に100件、理由つき）
//	PUT /admin/items/{id}/moderation         {"status": "ok"|"hidden", "note": "任意、120字"} → 200 更新後の商品
//
// note があれば moderation_reason を note で上書きする。status・note が決まりの外なら 400、無い商品は 404。
type AdminController struct {
	Usecase *usecase.AdminUsecase
	Auth    *Auth
}

func (c *AdminController) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /admin/items", c.Auth.RequireAdmin(c.list))
	mux.HandleFunc("PUT /admin/items/{id}/moderation", c.Auth.RequireAdmin(c.moderate))
}

func (c *AdminController) list(w http.ResponseWriter, r *http.Request, me model.Member) {
	items, err := c.Usecase.Queue(me, r.URL.Query().Get("status"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (c *AdminController) moderate(w http.ResponseWriter, r *http.Request, me model.Member) {
	var req model.ModerationUpdateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("fail: json.Decode, %v", err)
		writeError(w, http.StatusBadRequest, "invalid_input")
		return
	}
	it, err := c.Usecase.SetModeration(me, r.PathValue("id"), req)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, it)
}
