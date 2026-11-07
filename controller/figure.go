package controller

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"hackathon-backend/model"
	"hackathon-backend/usecase"
)

// FigureController は写真からデフォルメ3D模型の設計図（部品の JSON）を作る。
//
//	POST /figures  {"image": "<base64 または data:image/...;base64,...>", "name": "商品名（任意）"}
//	→ {"figure": {"name", "caption", "parts": [...], "face": {...}}}
//
// ログイン＋表示名の登録が必要。1人1分3回まで（429 too_many）。
// 画像は jpeg / png / webp、4MB まで（400 invalid_input）。Gemini の失敗や読めない出力は 502 ai_failed。
type FigureController struct {
	Usecase *usecase.FigureUsecase
	Auth    *Auth
}

func (c *FigureController) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /figures", c.Auth.RequireMember(c.create))
}

type figureReq struct {
	Image string `json:"image"`
	Name  string `json:"name"`
}

func (c *FigureController) create(w http.ResponseWriter, r *http.Request, me model.Member) {
	r.Body = http.MaxBytesReader(w, r.Body, usecase.FigureImageMax*4/3+64<<10) // base64 は 4/3 倍
	var req figureReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_input")
		return
	}
	b64 := req.Image
	if i := strings.Index(b64, ";base64,"); strings.HasPrefix(b64, "data:") && i >= 0 {
		b64 = b64[i+len(";base64,"):]
	}
	img, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_input")
		return
	}
	fig, err := c.Usecase.FromImage(r.Context(), me, img, req.Name)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]any{"figure": fig})
	case errors.Is(err, usecase.ErrInvalidInput), errors.Is(err, usecase.ErrTooMany):
		writeErr(w, err)
	default:
		log.Printf("fail: figure, %v", err)
		writeError(w, http.StatusBadGateway, "ai_failed")
	}
}
