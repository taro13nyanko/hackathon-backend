package controller

import (
	"encoding/json"
	"log"
	"net/http"

	"hackathon-backend/model"
	"hackathon-backend/usecase"
)

// MeController はログイン中の人の情報。
//
//	GET  /me   自分の情報（未登録なら 404 → 画面はプロフィール登録へ）
//	POST /me   プロフィール登録（初回ログイン時）
type MeController struct {
	Usecase *usecase.MemberUsecase
	Auth    *Auth
}

func (c *MeController) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /me", c.Auth.RequireLogin(c.get))
	mux.HandleFunc("POST /me", c.Auth.RequireLogin(c.create))
}

func (c *MeController) get(w http.ResponseWriter, r *http.Request, uid string) {
	me, err := c.Usecase.Me(uid)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, me)
}

// POST /me  {"display_name": "ota"}
func (c *MeController) create(w http.ResponseWriter, r *http.Request, uid string) {
	var req model.MemberCreateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("fail: json.Decode, %v", err)
		writeError(w, http.StatusBadRequest, "invalid_input")
		return
	}
	me, err := c.Usecase.Register(uid, req)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, me)
}
