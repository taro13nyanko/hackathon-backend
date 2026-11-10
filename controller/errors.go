package controller

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"hackathon-backend/model"
	"hackathon-backend/usecase"
)

// writeError はエラーを {"error": "<コード>"} で返す（コードの一覧は 設計/openapi.yaml の Error）。
func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code})
}

// writeErr は usecase / dao から返ってきたエラーを、HTTP のステータスとコードに変える。
func writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, usecase.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid_input")
	case errors.Is(err, usecase.ErrTooMany):
		writeError(w, http.StatusTooManyRequests, "too_many")
	case errors.Is(err, model.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found")
	case errors.Is(err, model.ErrAlreadyRegistered):
		writeError(w, http.StatusConflict, "already_registered")
	case errors.Is(err, model.ErrAlreadySold):
		writeError(w, http.StatusConflict, "already_sold")
	case errors.Is(err, model.ErrOwnItem):
		writeError(w, http.StatusConflict, "own_item")
	case errors.Is(err, model.ErrNoSeller):
		writeError(w, http.StatusConflict, "no_seller")
	case errors.Is(err, model.ErrImageQuota):
		writeError(w, http.StatusTooManyRequests, "image_quota")
	case errors.Is(err, model.ErrFigureExists):
		writeError(w, http.StatusConflict, "figure_exists")
	case errors.Is(err, model.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden")
	default:
		log.Printf("fail: %v", err)
		writeError(w, http.StatusInternalServerError, "internal")
	}
}

// writeJSON は v を JSON で返す。
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
