package controller

import (
	"io"
	"net/http"

	"hackathon-backend/model"
	"hackathon-backend/usecase"
)

// ImageController は画像の置き場所。
//
//	POST /images        画像を上げる（本文に画像そのもの。ログイン＋表示名が必要）→ 201 {"id": "..."}
//	GET  /images/{id}   画像を返す（だれでも。中身は変わらないので長くキャッシュさせる）
type ImageController struct {
	Usecase *usecase.ImageUsecase
	Auth    *Auth
}

func (c *ImageController) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /images", c.Auth.RequireMember(c.upload))
	mux.HandleFunc("GET /images/{id}", c.get)
}

func (c *ImageController) upload(w http.ResponseWriter, r *http.Request, me model.Member) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, usecase.ImageMaxBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_input") // 大きすぎる
		return
	}
	id, err := c.Usecase.Upload(me, data)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (c *ImageController) get(w http.ResponseWriter, r *http.Request) {
	img, err := c.Usecase.Get(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	h := w.Header()
	h.Set("Content-Type", img.ContentType)
	h.Set("Cache-Control", "public, max-age=31536000, immutable")
	h.Set("X-Content-Type-Options", "nosniff")
	w.Write(img.Data)
}
