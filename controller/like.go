package controller

import (
	"net/http"

	"hackathon-backend/model"
	"hackathon-backend/usecase"
)

// LikeController はいいね（どれもログイン＋表示名の登録が必要）。
//
//	PUT    /items/{id}/like   いいねする → 200 {"liked": true, "like_count": N}（2回目も成功）
//	DELETE /items/{id}/like   取り消す   → 200 {"liked": false, "like_count": N}（付いていなくても成功）
//	GET    /me/likes          いいねした商品（いいねした新しい順、100件、非公開の商品は除く）
//
// 非公開の商品・無い商品は 404。1人1分60回まで（429 too_many）。
type LikeController struct {
	Usecase *usecase.LikeUsecase
	Auth    *Auth
}

func (c *LikeController) Register(mux *http.ServeMux) {
	mux.HandleFunc("PUT /items/{id}/like", c.Auth.RequireMember(c.like))
	mux.HandleFunc("DELETE /items/{id}/like", c.Auth.RequireMember(c.unlike))
	mux.HandleFunc("GET /me/likes", c.Auth.RequireMember(c.list))
}

func (c *LikeController) like(w http.ResponseWriter, r *http.Request, me model.Member) {
	res, err := c.Usecase.Like(me, r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (c *LikeController) unlike(w http.ResponseWriter, r *http.Request, me model.Member) {
	res, err := c.Usecase.Unlike(me, r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (c *LikeController) list(w http.ResponseWriter, r *http.Request, me model.Member) {
	items, err := c.Usecase.List(me)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}
