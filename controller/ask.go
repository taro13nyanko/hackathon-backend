package controller

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"hackathon-backend/model"
	"hackathon-backend/usecase"
)

// AskController は商品への質問（自前の RAG。商品の情報とアプリの決まりだけを根拠に Gemini が答える）。
//
//	POST /items/{id}/ask  {"question": "1〜200字"}
//	→ 200 {"answer": "…[1]…", "sources": [{"id": 1, "kind": "item", "title": "この商品の説明", "text": "…"}], "grounded": true}
//
// ログイン＋表示名の登録が必要。1人1分5回まで（429 too_many）。質問が空・長すぎるなら 400 invalid_input、
// 無い商品・非公開の商品は 404。Gemini の失敗や読めない出力は 502 ai_failed。
type AskController struct {
	Usecase *usecase.AskUsecase
	Auth    *Auth
}

func (c *AskController) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /items/{id}/ask", c.Auth.RequireMember(c.ask))
}

func (c *AskController) ask(w http.ResponseWriter, r *http.Request, me model.Member) {
	var req usecase.AskReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_input")
		return
	}
	res, err := c.Usecase.Ask(r.Context(), me, r.PathValue("id"), req)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, res)
	case errors.Is(err, usecase.ErrInvalidInput), errors.Is(err, usecase.ErrTooMany), errors.Is(err, model.ErrNotFound):
		writeErr(w, err)
	default:
		log.Printf("fail: ask, %v", err)
		writeError(w, http.StatusBadGateway, "ai_failed")
	}
}
