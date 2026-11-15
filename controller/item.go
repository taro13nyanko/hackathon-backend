package controller

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	"hackathon-backend/model"
	"hackathon-backend/usecase"
)

type ItemController struct {
	Usecase *usecase.ItemUsecase
	Auth    *Auth
}

// Register は商品まわりのURLを mux に登録する（Go 1.22 以降の「メソッド＋パス」書式）。
//
//	GET  /items          一覧（?q=キーワード、?min_price=・?max_price= で絞り込み。非公開の商品は出さない）
//	POST /items          出品（ログイン＋プロフィール登録が必要）。保存の前に AI が確認し、明らかな違反は 422 prohibited
//	GET  /items/{id}     詳細（非公開の商品は出品者本人と管理者だけ。ほかは 404）
//
// 一覧と詳細はログイン不要。Authorization があって有効なら本人として liked と確認の理由を入れる（OptionalLogin）。
func (c *ItemController) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /items", c.Auth.OptionalLogin(c.list))
	mux.HandleFunc("POST /items", c.Auth.RequireMember(c.create))
	mux.HandleFunc("GET /items/{id}", c.Auth.OptionalLogin(c.get))
}

// GET /items?q=キーワード&min_price=700&max_price=1300（どれも任意。値段は 0 や空なら指定なし、数字でなければ 400）
func (c *ItemController) list(w http.ResponseWriter, r *http.Request, me *model.Member) {
	query := r.URL.Query()
	minPrice, err1 := priceParam(query.Get("min_price"))
	maxPrice, err2 := priceParam(query.Get("max_price"))
	if err1 != nil || err2 != nil {
		writeError(w, http.StatusBadRequest, "invalid_input")
		return
	}
	items, err := c.Usecase.List(me, model.ItemQuery{Keyword: query.Get("q"), MinPrice: minPrice, MaxPrice: maxPrice})
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// priceParam は値段の検索条件を読む。空は 0（指定なし）。数字でなければエラー（範囲は usecase が確かめる）。
func priceParam(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	return strconv.Atoi(s)
}

// POST /items {"title": "...", "description": "...", "price": 1200}
// → 201 {"id", "moderation_status", "moderation_reason"}
func (c *ItemController) create(w http.ResponseWriter, r *http.Request, me model.Member) {
	var req model.ItemCreateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("fail: json.Decode, %v", err)
		writeError(w, http.StatusBadRequest, "invalid_input")
		return
	}
	res, err := c.Usecase.Create(r.Context(), me, req)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

func (c *ItemController) get(w http.ResponseWriter, r *http.Request, me *model.Member) {
	it, err := c.Usecase.Get(me, r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, it)
}
