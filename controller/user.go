package controller

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"hackathon-backend/model"
	"hackathon-backend/usecase"
)

type UserController struct {
	Usecase *usecase.UserUsecase
}

// ServeHTTP は /user をメソッドごとに振り分ける。
func (c *UserController) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		c.get(w, r)
	case http.MethodPost:
		c.post(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// GET /user?name=xxx
func (c *UserController) get(w http.ResponseWriter, r *http.Request) {
	users, err := c.Usecase.Search(r.URL.Query().Get("name"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, users)
}

// POST /user  {"name": "...", "age": 20}
func (c *UserController) post(w http.ResponseWriter, r *http.Request) {
	var req model.UserCreateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("fail: json.Decode, %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	res, err := c.Usecase.Register(req)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// ListHandler は GET /users（全件取得）。
func (c *UserController) ListHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	users, err := c.Usecase.List()
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func writeErr(w http.ResponseWriter, err error) {
	if errors.Is(err, usecase.ErrInvalidInput) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	log.Printf("fail: %v", err)
	w.WriteHeader(http.StatusInternalServerError)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		log.Printf("fail: json.Marshal, %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(b)
}
