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
//	仕上げ：        上に加えて "previous": 前回の設計図, "render": 前回の設計図を描いた画像（data URL）
//	→ {"figure": {"name", "caption", "parts": [...], "face": {...}}}
//
// ログイン＋表示名の登録が必要。1人1分6回まで（429 too_many）。
// 画像は jpeg / png / webp、4MB まで（400 invalid_input）。Gemini の失敗や読めない出力は 502 ai_failed。
//
// 保存：PUT /items/{id}/figure  {"figure": {...}} → 200 {"figure": 直した設計図}
// 出品者はいつでも（上書き）、購入者は模型がまだ無いときだけ（あれば 409 figure_exists）、ほかの人は 403。
type FigureController struct {
	Usecase *usecase.FigureUsecase
	Items   *usecase.ItemUsecase // 模型の保存用
	Auth    *Auth
}

func (c *FigureController) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /figures", c.Auth.RequireMember(c.create))
	if c.Items != nil {
		mux.HandleFunc("PUT /items/{id}/figure", c.Auth.RequireMember(c.save))
	}
}

func (c *FigureController) save(w http.ResponseWriter, r *http.Request, me model.Member) {
	r.Body = http.MaxBytesReader(w, r.Body, usecase.FigureJSONMax*2)
	var req struct {
		Figure *usecase.Figure `json:"figure"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Figure == nil {
		writeError(w, http.StatusBadRequest, "invalid_input")
		return
	}
	fig, err := c.Items.SaveFigure(me, r.PathValue("id"), req.Figure)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"figure": fig})
}

type figureReq struct {
	Image    string          `json:"image"`
	Name     string          `json:"name"`
	Previous *usecase.Figure `json:"previous"` // 仕上げのとき：前回の設計図
	Render   string          `json:"render"`   // 仕上げのとき：前回の設計図を描いた画像
}

// decodeImage は base64 か data URL を読む。空なら nil。
func decodeImage(s string) ([]byte, error) {
	if s == "" {
		return nil, nil
	}
	if i := strings.Index(s, ";base64,"); strings.HasPrefix(s, "data:") && i >= 0 {
		s = s[i+len(";base64,"):]
	}
	return base64.StdEncoding.DecodeString(s)
}

func (c *FigureController) create(w http.ResponseWriter, r *http.Request, me model.Member) {
	r.Body = http.MaxBytesReader(w, r.Body, usecase.FigureImageMax*3+256<<10) // 画像2枚（base64 は 4/3 倍）＋設計図
	var req figureReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_input")
		return
	}
	img, err := decodeImage(req.Image)
	render, err2 := decodeImage(req.Render)
	if err != nil || err2 != nil {
		writeError(w, http.StatusBadRequest, "invalid_input")
		return
	}
	fig, err := c.Usecase.Make(r.Context(), me, usecase.FigureInput{Photo: img, Name: req.Name, Previous: req.Previous, Render: render})
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
