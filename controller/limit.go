package controller

import "net/http"

// BodyMax は JSON を受ける API の本文の上限（模型の保存 64KB を含めて十分な大きさ）。
const BodyMax = 256 << 10

// LimitBody はすべての要求の本文を BodyMax までに制限する。
// 画像（POST /images）と模型づくり（POST /figures）は大きな画像を受けるので、各ハンドラが自分の上限を付ける。
func LimitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil && !(r.Method == http.MethodPost && (r.URL.Path == "/images" || r.URL.Path == "/figures")) {
			r.Body = http.MaxBytesReader(w, r.Body, BodyMax)
		}
		next.ServeHTTP(w, r)
	})
}
