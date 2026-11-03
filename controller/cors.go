package controller

import (
	"net/http"
	"strings"
)

// CORS は Vercel のフロントエンドからのリクエストを許可する。
//
// ALLOWED_ORIGINS にカンマ区切りで許可するオリジンを書く。
//
//	例: https://hackathon-frontend.vercel.app,http://localhost:5173
//
// 末尾の "/" は付けない。未設定なら "*"（全部許可。開発中だけにすること）。
func CORS(allowed string, next http.Handler) http.Handler {
	origins := map[string]bool{}
	for _, o := range strings.Split(allowed, ",") {
		if o = strings.TrimRight(strings.TrimSpace(o), "/"); o != "" {
			origins[o] = true
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		switch {
		case len(origins) == 0:
			w.Header().Set("Access-Control-Allow-Origin", "*")
		case origins[origin]:
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		// ブラウザは POST(JSON) の前に OPTIONS（プリフライト）を送ってくる
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
