package controller

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"hackathon-backend/model"
	"hackathon-backend/usecase"
)

// DescriptionController は Gemini で説明文の下書きを作る。
//
//	POST /items/description  {"title": "...", "notes": "..."} → {"description": "..."}
//
// ログイン＋表示名の登録が必要。1人1分5回まで（超えたら 429 too_many）。
// Gemini 側の失敗は 502 ai_failed（画面は「もう一度」を出す）。
type DescriptionController struct {
	Usecase *usecase.DescriptionUsecase
	Auth    *Auth
}

func (c *DescriptionController) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /items/description", c.Auth.RequireMember(c.draft))
}

func (c *DescriptionController) draft(w http.ResponseWriter, r *http.Request, me model.Member) {
	var req usecase.DescriptionReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_input")
		return
	}
	text, err := c.Usecase.Draft(r.Context(), me, req)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]string{"description": text})
	case errors.Is(err, usecase.ErrInvalidInput), errors.Is(err, usecase.ErrTooMany):
		writeErr(w, err)
	default:
		log.Printf("fail: gemini, %v", err)
		writeError(w, http.StatusBadGateway, "ai_failed")
	}
}
