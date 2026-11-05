package controller

import (
	"net/http"

	"hackathon-backend/model"
	"hackathon-backend/usecase"
)

// PurchaseController は購入とマイページ。
//
//	POST /items/{id}/purchase   購入する（決済なし）。売り切れ・自分の商品は 409
//	GET  /me/items              自分が出品した商品
//	GET  /me/purchases          自分が買った商品
//
// どれもログイン＋表示名の登録が必要。
type PurchaseController struct {
	Purchases *usecase.PurchaseUsecase
	Items     *usecase.ItemUsecase
	Auth      *Auth
}

func (c *PurchaseController) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /items/{id}/purchase", c.Auth.RequireMember(c.buy))
	mux.HandleFunc("GET /me/items", c.Auth.RequireMember(c.myItems))
	mux.HandleFunc("GET /me/purchases", c.Auth.RequireMember(c.myPurchases))
}

func (c *PurchaseController) buy(w http.ResponseWriter, r *http.Request, me model.Member) {
	res, err := c.Purchases.Buy(me, r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

func (c *PurchaseController) myItems(w http.ResponseWriter, r *http.Request, me model.Member) {
	items, err := c.Items.Mine(me)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (c *PurchaseController) myPurchases(w http.ResponseWriter, r *http.Request, me model.Member) {
	ps, err := c.Purchases.History(me)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ps)
}
