package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"hackathon-backend/controller"
	"hackathon-backend/dao"
	"hackathon-backend/usecase"
)

func main() {
	db, err := dao.Open()
	if err != nil {
		log.Fatalf("fail: DB 接続, %v", err)
	}
	defer db.Close()

	userCtrl := &controller.UserController{
		Usecase: &usecase.UserUsecase{Store: &dao.UserDao{DB: db}},
	}

	itemCtrl := &controller.ItemController{
		Usecase: &usecase.ItemUsecase{Store: &dao.ItemDao{DB: db}},
	}

	mux := http.NewServeMux()
	itemCtrl.Register(mux) // GET/POST /items, GET /items/{id}
	mux.Handle("/user", userCtrl)
	mux.HandleFunc("/users", userCtrl.ListHandler)
	// 動作確認用（DB まで届くか見る）。Cloud Run では z で終わるパス（/healthz）は予約されていて届かないので /health にする
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		if err := db.PingContext(r.Context()); err != nil {
			log.Printf("fail: health ping, %v", err)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte("ok"))
	})

	// Cloud Run は PORT 環境変数（通常 8080）で待ち受けポートを渡してくる
	port := os.Getenv("PORT")
	if port == "" {
		port = "8000"
	}
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           controller.CORS(os.Getenv("ALLOWED_ORIGINS"), mux),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Ctrl+C / Cloud Run の停止(SIGTERM) で、処理中のリクエストを終えてから DB を閉じる
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		log.Println("shutting down...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("fail: Shutdown, %v", err)
		}
	}()

	log.Printf("Listening on :%s", port)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	log.Println("success: server stopped, closing DB")
}
