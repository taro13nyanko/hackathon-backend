package controller

import (
	"encoding/json"
	"net/http"

	"hackathon-backend/model"
	"hackathon-backend/usecase"
)

// GardenController は箱庭の配置（模型をつかんで動かした場所）。
//
//	GET /me/garden   自分の配置 → {"spots": [{"item_id","x","z"}]}
//	PUT /me/garden   配置をまとめて保存（前の配置は消える。空の spots で「並べ直す」）
//
// どちらもログイン＋表示名の登録が必要。自分の箱庭に無い商品の位置は 403。
type GardenController struct {
	Usecase *usecase.GardenUsecase
	Auth    *Auth
}

func (c *GardenController) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /me/garden", c.Auth.RequireMember(c.get))
	mux.HandleFunc("PUT /me/garden", c.Auth.RequireMember(c.save))
}

func (c *GardenController) get(w http.ResponseWriter, r *http.Request, me model.Member) {
	layout, err := c.Usecase.Get(me)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, layout)
}

func (c *GardenController) save(w http.ResponseWriter, r *http.Request, me model.Member) {
	var req model.GardenLayout
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_input")
		return
	}
	layout, err := c.Usecase.Save(me, req)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, layout)
}
