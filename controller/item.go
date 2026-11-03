package controller

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"hackathon-backend/model"
	"hackathon-backend/usecase"
)

type ItemController struct {
	Usecase *usecase.ItemUsecase
}

// Register は商品まわりのURLを mux に登録する（Go 1.22 以降の「メソッド＋パス」書式）。
//
//	GET  /items          一覧（?q=キーワード で絞り込み）
//	POST /items          出品
//	GET  /items/{id}     詳細
func (c *ItemController) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /items", c.list)
	mux.HandleFunc("POST /items", c.create)
	mux.HandleFunc("GET /items/{id}", c.get)
}

func (c *ItemController) list(w http.ResponseWriter, r *http.Request) {
	items, err := c.Usecase.List(r.URL.Query().Get("q"))
	if err != nil {
		writeItemErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// POST /items  {"title": "...", "description": "...", "price": 1200}
func (c *ItemController) create(w http.ResponseWriter, r *http.Request) {
	var req model.ItemCreateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("fail: json.Decode, %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	res, err := c.Usecase.Create(req)
	if err != nil {
		writeItemErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

func (c *ItemController) get(w http.ResponseWriter, r *http.Request) {
	it, err := c.Usecase.Get(r.PathValue("id"))
	if err != nil {
		writeItemErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, it)
}

// writeItemErr は writeErr（400/500）に 404 を足したもの。
func writeItemErr(w http.ResponseWriter, err error) {
	if errors.Is(err, model.ErrNotFound) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	writeErr(w, err)
}
