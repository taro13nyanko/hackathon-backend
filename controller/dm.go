package controller

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"

	"hackathon-backend/model"
	"hackathon-backend/usecase"
)

// DMController はユーザー間のメッセージ（どれもログイン＋表示名の登録が必要）。
//
//	POST /items/{id}/conversations        この商品の出品者とのやりとりを始める（あれば 200 で既存、無ければ 201）
//	GET  /conversations                   自分のやりとり一覧（最新メッセージが新しい順、それぞれ unread_count つき）
//	GET  /conversations/{id}/messages     メッセージ（古い順）。?after=<メッセージID> でそれより新しいものだけ
//	POST /conversations/{id}/messages     送る {"body": "..."}（1〜1000文字）
//	PUT  /conversations/{id}/read         既読にする {"last_message_id": "任意"} → {"last_read_message_id","unread_count"}
//	GET  /me/unread                       自分の未読の合計 → {"count": N}（ナビの 💬 用）
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
	mux.HandleFunc("PUT /conversations/{id}/read", c.Auth.RequireMember(c.read))
	mux.HandleFunc("GET /me/unread", c.Auth.RequireMember(c.unread))
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

// read は既読にする。ボディは省略できる（空なら最新まで既読）。
func (c *DMController) read(w http.ResponseWriter, r *http.Request, me model.Member) {
	var req model.ReadReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		log.Printf("fail: json.Decode, %v", err)
		writeError(w, http.StatusBadRequest, "invalid_input")
		return
	}
	res, err := c.Usecase.MarkRead(me, r.PathValue("id"), req)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (c *DMController) unread(w http.ResponseWriter, r *http.Request, me model.Member) {
	res, err := c.Usecase.Unread(me)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
