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

	// ログイン（Firebase の ID トークンを確かめる）。プロジェクト ID は環境変数で変えられる
	projectID := os.Getenv("FIREBASE_PROJECT_ID")
	if projectID == "" {
		projectID = "uttc-c10-yusuke-ota-5da712"
	}
	members := &usecase.MemberUsecase{Store: &dao.MemberDao{DB: db}}
	authz := &controller.Auth{Verifier: auth.NewVerifier(projectID), Members: members}

	images := &dao.ImageDao{DB: db}
	itemDao := &dao.ItemDao{DB: db}
	// 回数制限（1人あたり。サーバー1台のメモリ内で数える）。画像は合計枚数も DB で数える（usecase/image.go）
	// 出品の確認（禁止物・不適切な画像・写真と説明の食い違い）。説明文と同じ軽いモデルで、出力の形（JSON Schema）を決めて判定させる
	moderator := &usecase.GeminiModerator{Gen: moderationClient(projectID)}
	items := &usecase.ItemUsecase{Store: itemDao, Images: images, Figures: itemDao, Moderator: moderator, Limit: usecase.NewLimiter(10, time.Minute)}
	imageCtrl := &controller.ImageController{Usecase: &usecase.ImageUsecase{Store: images, Limit: usecase.NewLimiter(10, time.Minute)}, Auth: authz}
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
		Items: items,
		Auth:  authz,
	}
	dmCtrl := &controller.DMController{
		Usecase: &usecase.DMUsecase{Store: &dao.DMDao{DB: db}, Items: &dao.ItemDao{DB: db}, Limit: usecase.NewLimiter(30, time.Minute)},
		Auth:    authz,
	}

	// いいね（1人1分60回まで）と、管理者の画面（要確認・非公開の商品）
	likeCtrl := &controller.LikeController{
		Usecase: &usecase.LikeUsecase{Store: &dao.LikeDao{DB: db}, Items: itemDao, Limit: usecase.NewLimiter(usecase.LikePerMinute, time.Minute)},
		Auth:    authz,
	}
	adminCtrl := &controller.AdminController{Usecase: &usecase.AdminUsecase{Store: itemDao, Items: itemDao}, Auth: authz}

	// 音声検索（1人1分10回まで）と商品への質問（自前の RAG、1人1分5回まで）。どちらも説明文と同じ軽いモデルで、出力の形を決める
	voiceCtrl := &controller.VoiceController{
		Usecase: &usecase.VoiceUsecase{
			Gen: &gemini.Client{
				Project:    envOr("GOOGLE_CLOUD_PROJECT", projectID),
				Location:   envOr("GOOGLE_CLOUD_LOCATION", "global"),
				Model:      envOr("GEMINI_MODEL", "gemini-3.5-flash-lite"),
				JSONSchema: usecase.VoiceSchema,
			},
			Limit: usecase.NewLimiter(usecase.VoicePerMinute, time.Minute),
		},
		Auth: authz,
	}
	askCtrl := &controller.AskController{
		Usecase: &usecase.AskUsecase{
			Items: itemDao,
			Gen: &gemini.Client{
				Project:    envOr("GOOGLE_CLOUD_PROJECT", projectID),
				Location:   envOr("GOOGLE_CLOUD_LOCATION", "global"),
				Model:      envOr("GEMINI_MODEL", "gemini-3.5-flash-lite"),
				JSONSchema: usecase.AskSchema,
			},
			Limit: usecase.NewLimiter(usecase.AskPerMinute, time.Minute),
		},
		Auth: authz,
	}

	// 箱庭の配置（模型をつかんで動かした場所）
	gardenCtrl := &controller.GardenController{
		Usecase: &usecase.GardenUsecase{Store: &dao.GardenDao{DB: db}, Limit: usecase.NewLimiter(30, time.Minute)},
		Auth:    authz,
	}

	mux := http.NewServeMux()
	itemCtrl.Register(mux)     // GET/POST /items, GET /items/{id}
	likeCtrl.Register(mux)     // PUT/DELETE /items/{id}/like, GET /me/likes
	adminCtrl.Register(mux)    // GET /admin/items, PUT /admin/items/{id}/moderation
	voiceCtrl.Register(mux)    // POST /search/voice
	askCtrl.Register(mux)      // POST /items/{id}/ask
	meCtrl.Register(mux)       // GET/POST /me
	purchaseCtrl.Register(mux) // POST /items/{id}/purchase, GET /me/items, GET /me/purchases
	descCtrl.Register(mux)     // POST /items/description
	figureCtrl.Register(mux)   // POST /figures, PUT /items/{id}/figure
	imageCtrl.Register(mux)    // POST /images, GET /images/{id}
	gardenCtrl.Register(mux)   // GET/PUT /me/garden
	dmCtrl.Register(mux)       // POST /items/{id}/conversations, GET /conversations, GET/POST /conversations/{id}/messages
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
		Handler:           controller.CORS(os.Getenv("ALLOWED_ORIGINS"), controller.LimitBody(mux)),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,  // 本文を送り終えるまで（模型づくりは画像2枚で最大12MB ほど）
		WriteTimeout:      300 * time.Second, // 返し終えるまで（模型づくりは Gemini を最大240秒待つ）
		IdleTimeout:       120 * time.Second,
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

// moderationSeed は出品の確認で使う乱数の種（固定）。
const moderationSeed = 1

// moderationClient は出品の確認用の Gemini Client を作る。
// 同じ入力で判定がばらつかないよう乱数の種を固定し、小さな欠けも見えるよう画像は細かく読ませる（この Client だけ）。
func moderationClient(projectID string) *gemini.Client {
	seed := moderationSeed
	return &gemini.Client{
		Project:         envOr("GOOGLE_CLOUD_PROJECT", projectID),
		Location:        envOr("GOOGLE_CLOUD_LOCATION", "global"),
		Model:           envOr("GEMINI_MODEL", "gemini-3.5-flash-lite"),
		JSONSchema:      usecase.ModerationSchema,
		Seed:            &seed,
		MediaResolution: "MEDIA_RESOLUTION_HIGH",
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
