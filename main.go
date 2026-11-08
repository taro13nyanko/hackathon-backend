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

	"hackathon-backend/auth"
	"hackathon-backend/controller"
	"hackathon-backend/dao"
	"hackathon-backend/gemini"
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

	// ログイン（Firebase の ID トークンを確かめる）。プロジェクト ID は環境変数で変えられる
	projectID := os.Getenv("FIREBASE_PROJECT_ID")
	if projectID == "" {
		projectID = "uttc-c10-yusuke-ota-5da712"
	}
	members := &usecase.MemberUsecase{Store: &dao.MemberDao{DB: db}}
	authz := &controller.Auth{Verifier: auth.NewVerifier(projectID), Members: members}

	images := &dao.ImageDao{DB: db}
	items := &usecase.ItemUsecase{Store: &dao.ItemDao{DB: db}, Images: images}
	imageCtrl := &controller.ImageController{Usecase: &usecase.ImageUsecase{Store: images}, Auth: authz}
	itemCtrl := &controller.ItemController{Usecase: items, Auth: authz}
	meCtrl := &controller.MeController{Usecase: members, Auth: authz}
	purchaseCtrl := &controller.PurchaseController{
		Purchases: &usecase.PurchaseUsecase{Store: &dao.PurchaseDao{DB: db}},
		Items:     items,
		Auth:      authz,
	}
	// Gemini（Vertex AI）。モデルと場所は環境変数で変えられる
	descCtrl := &controller.DescriptionController{
		Usecase: &usecase.DescriptionUsecase{Gen: &gemini.Client{
			Project:  envOr("GOOGLE_CLOUD_PROJECT", projectID),
			Location: envOr("GOOGLE_CLOUD_LOCATION", "global"),
			Model:    envOr("GEMINI_MODEL", "gemini-3.5-flash-lite"),
		}},
		Auth: authz,
	}
	// 写真 → デフォルメ3D模型の設計図（説明文と同じ Gemini を使う）
	// 模型づくりは使える中で最上位のモデル（FIGURE_MODEL、Gemini 4 Argon・考える深さ medium。high は3分を超えて間に合わなかった）で作らせ、失敗したら説明文と同じ軽いモデルでやり直す
	figureCtrl := &controller.FigureController{
		Usecase: &usecase.FigureUsecase{
			Gen: &gemini.Client{
				Project:       envOr("GOOGLE_CLOUD_PROJECT", projectID),
				Location:      envOr("GOOGLE_CLOUD_LOCATION", "global"),
				Model:         envOr("FIGURE_MODEL", "gemini-4-argon"),
				ThinkingLevel: envOr("FIGURE_THINKING", "medium"),
				JSONSchema:    usecase.FigureSchema,
			},
			Fallback: &gemini.Client{
				Project:    envOr("GOOGLE_CLOUD_PROJECT", projectID),
				Location:   envOr("GOOGLE_CLOUD_LOCATION", "global"),
				Model:      envOr("GEMINI_MODEL", "gemini-3.5-flash-lite"),
				JSONSchema: usecase.FigureSchema,
			},
		},
		Auth: authz,
	}
	dmCtrl := &controller.DMController{
		Usecase: &usecase.DMUsecase{Store: &dao.DMDao{DB: db}, Items: &dao.ItemDao{DB: db}},
		Auth:    authz,
	}

	mux := http.NewServeMux()
	itemCtrl.Register(mux)     // GET/POST /items, GET /items/{id}
	meCtrl.Register(mux)       // GET/POST /me
	purchaseCtrl.Register(mux) // POST /items/{id}/purchase, GET /me/items, GET /me/purchases
	descCtrl.Register(mux)     // POST /items/description
	figureCtrl.Register(mux)   // POST /figures
	imageCtrl.Register(mux)    // POST /images, GET /images/{id}
	dmCtrl.Register(mux)       // POST /items/{id}/conversations, GET /conversations, GET/POST /conversations/{id}/messages
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

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
