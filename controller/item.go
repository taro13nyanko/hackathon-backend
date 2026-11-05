package controller

import (
	"encoding/json"
	"log"
	"net/http"

	"hackathon-backend/model"
	"hackathon-backend/usecase"
)

type ItemController struct {
	Usecase *usecase.ItemUsecase
	Auth    *Auth
}

// Register は商品まわりのURLを mux に登録する（Go 1.22 以降の「メソッド＋パス」書式）。
//
//	GET  /items          一覧（?q=キーワード で絞り込み）
//	POST /items          出品（ログイン＋プロフィール登録が必要）
//	GET  /items/{id}     詳細
func (c *ItemController) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /items", c.list)
	mux.HandleFunc("POST /items", c.Auth.RequireMember(c.create))
	mux.HandleFunc("GET /items/{id}", c.get)
}

func (c *ItemController) list(w http.ResponseWriter, r *http.Request) {
	items, err := c.Usecase.List(r.URL.Query().Get("q"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// POST /items  {"title": "...", "description": "...", "price": 1200}
func (c *ItemController) create(w http.ResponseWriter, r *http.Request, me model.Member) {
	var req model.ItemCreateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("fail: json.Decode, %v", err)
		writeError(w, http.StatusBadRequest, "invalid_input")
		return
	}
	res, err := c.Usecase.Create(me, req)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

func (c *ItemController) get(w http.ResponseWriter, r *http.Request) {
	it, err := c.Usecase.Get(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, it)
}
