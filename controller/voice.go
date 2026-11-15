package controller

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"hackathon-backend/model"
	"hackathon-backend/usecase"
)

// VoiceController は音声検索（録音した音声を Gemini で検索の条件に直す）。
//
//	POST /search/voice  {"audio": "<base64 または data:audio/...;base64,...>", "mime": "audio/webm"}
//	→ 200 {"transcript": "話した内容", "q": "検索語（空可）", "min_price": 0, "max_price": 3000}（値段の 0 は指定なし）
//
// ログイン＋表示名の登録が必要。1人1分10回まで（429 too_many）。
// 音声は 1MB まで、mime は audio/webm・ogg・mp4・mpeg・wav だけ（400 invalid_input）。Gemini の失敗や読めない出力は 502 ai_failed。
type VoiceController struct {
	Usecase *usecase.VoiceUsecase
	Auth    *Auth
}

func (c *VoiceController) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /search/voice", c.Auth.RequireMember(c.search))
}

// VoiceBodyMax は POST /search/voice の本文の上限（音声 1MB の base64（4/3 倍）＋ mime などの小さな余白）。
// 共通の上限（LimitBody）の対象外にして、ここで付ける。
const VoiceBodyMax = usecase.VoiceAudioMax*4/3 + 4<<10

func (c *VoiceController) search(w http.ResponseWriter, r *http.Request, me model.Member) {
	r.Body = http.MaxBytesReader(w, r.Body, VoiceBodyMax)
	var req struct {
		Audio string `json:"audio"`
		Mime  string `json:"mime"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_input")
		return
	}
	audio, err := decodeImage(req.Audio) // base64 か data URL（画像と同じ読み方）
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_input")
		return
	}
	res, err := c.Usecase.Search(r.Context(), me, usecase.VoiceInput{Audio: audio, Mime: req.Mime})
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, res)
	case errors.Is(err, usecase.ErrInvalidInput), errors.Is(err, usecase.ErrTooMany):
		writeErr(w, err)
	default:
		log.Printf("fail: voice search, %v", err)
		writeError(w, http.StatusBadGateway, "ai_failed")
	}
}
