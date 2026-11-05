package controller

import (
	"encoding/json"
	"log"
	"net/http"

	"hackathon-backend/model"
	"hackathon-backend/usecase"
)

// DMController はユーザー間のメッセージ（どれもログイン＋表示名の登録が必要）。
//
//	POST /items/{id}/conversations        この商品の出品者とのやりとりを始める（あれば 200 で既存、無ければ 201）
//	GET  /conversations                   自分のやりとり一覧（最新メッセージが新しい順）
//	GET  /conversations/{id}/messages     メッセージ（古い順）。?after=<メッセージID> でそれより新しいものだけ
//	POST /conversations/{id}/messages     送る {"body": "..."}（1〜1000文字）
//
// 参加者（買い手と出品者）以外は 403。
type DMController struct {
	Usecase *usecase.DMUsecase
	Auth    *Auth
}

func (c *DMController) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /items/{id}/conversations", c.Auth.RequireMember(c.start))
	mux.HandleFunc("GET /conversations", c.Auth.RequireMember(c.list))
	mux.HandleFunc("GET /conversations/{id}/messages", c.Auth.RequireMember(c.messages))
	mux.HandleFunc("POST /conversations/{id}/messages", c.Auth.RequireMember(c.send))
}

func (c *DMController) start(w http.ResponseWriter, r *http.Request, me model.Member) {
	conv, created, err := c.Usecase.Start(me, r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, conv)
}

func (c *DMController) list(w http.ResponseWriter, r *http.Request, me model.Member) {
	cs, err := c.Usecase.List(me)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cs)
}

func (c *DMController) messages(w http.ResponseWriter, r *http.Request, me model.Member) {
	ms, err := c.Usecase.Messages(me, r.PathValue("id"), r.URL.Query().Get("after"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ms)
}

func (c *DMController) send(w http.ResponseWriter, r *http.Request, me model.Member) {
	var req model.MessageCreateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("fail: json.Decode, %v", err)
		writeError(w, http.StatusBadRequest, "invalid_input")
		return
	}
	m, err := c.Usecase.Send(me, r.PathValue("id"), req)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, m)
}
