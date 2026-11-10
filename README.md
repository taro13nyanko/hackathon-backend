# hackathon-backend

UTTC ハッカソン用のバックエンド（Go）。Cloud Run で動かし、Cloud SQL（MySQL 8.0）に接続する。
カリキュラムの `db` ブランチのコードを、controller / usecase / dao / model の4層に分けたもの。

| パス | 中身 |
|---|---|
| `main.go` | 起動・ルーティング・終了処理（SIGTERM で処理中のリクエストを終えてから止まる） |
| `gemini/` | Vertex AI の Gemini を REST で呼ぶ（標準ライブラリだけ。認証は Cloud Run のサービスアカウント） |
| `auth/` | Firebase の ID トークンの確認（署名・期限・宛先。標準ライブラリだけ） |
| `controller/` | HTTP の受け口（`/me`・`/items` ほか）、ログイン確認、CORS |
| `usecase/` | 入力チェック（表示名・商品）と ID 採番（ULID） |
| `dao/` | SQL と DB 接続（接続情報はすべて環境変数） |
| `model/` | 構造体 |
| `sql/schema.sql` | `user`・`item` テーブルの CREATE 文と見本データ |
| `sql/002_users.sql` | ログイン機能：`users` テーブルと `item.seller_id` |
| `sql/003_purchases.sql` | 購入機能：`purchases` テーブル（1商品1回まで） |
| `sql/004_dm.sql` | DM 機能：`conversations`・`messages` テーブル |

## API

本番: https://hackathon-backend-646017661953.us-central1.run.app

🔒 は `Authorization: Bearer <Firebase の ID トークン>` が必要（無い・無効なら 401）。
エラーは `{"error":"<コード>"}`（`invalid_input` `unauthorized` `profile_required` `not_found` `already_registered` `already_sold` `own_item` `no_seller` `forbidden` `too_many` `ai_failed` `internal`）。

| メソッド | パス | 内容 |
|---|---|---|
| GET | `/me` | 🔒 自分の `{"id","display_name"}`。表示名が未登録なら 404（画面はプロフィール登録へ） |
| POST | `/me` | 🔒 `{"display_name":"ota"}`（1〜50文字）で登録し 201。登録済みなら 409 |
| GET | `/me/items` | 🔒 自分が出品した商品（新しい順） |
| GET | `/me/purchases` | 🔒 自分が買った商品 `[{"id","item","price","created_at"}]`（新しい順） |
| GET | `/items` | 商品一覧（新しい順）。`?q=キーワード` でタイトル・説明を部分一致検索。0件なら `[]` |
| POST | `/items` | 🔒 表示名の登録が必要（未登録は 403 `profile_required`）。`{"title":"...","description":"...","price":1200}` で出品し、201 と `{"id":"..."}` を返す。商品名1〜40文字・説明1000文字まで・価格300〜9,999,999円（違反は 400） |
| POST | `/items/description` | 🔒 Gemini で説明文の下書き。`{"title":"...","notes":"メモ"}`（商品名1〜40文字、メモ500文字まで）→ `{"description":"..."}`。メモに書かれた事実だけで300字程度。1人1分5回まで（超えたら 429）、Gemini の失敗は 502 `ai_failed` |
| POST | `/figures` | 🔒 写真からデフォルメ3D模型の設計図を作る（箱庭用）。`{"image":"<base64 か data URL>","name":"商品名（任意）"}` → `{"figure":{"name","caption","parts":[...],"face":{...}}}`。画像は jpeg/png/webp・4MB まで。部品は40個まで、形・大きさ・色は範囲内に直して返す。1人1分3回まで（429）、Gemini の失敗や読めない出力は 502 `ai_failed` |
| PUT | `/items/{id}/figure` | 🔒 模型の設計図を商品に保存する（箱庭に並ぶ）。`{"figure":{...POST /figures の返り値...}}` → `{"figure":直した設計図}`。出品者はいつでも上書きでき、購入者は模型がまだ無いときだけ（あれば 409 `figure_exists`）、ほかの人は 403。保存前に POST /figures と同じ決まりで範囲内に直す（64KB まで）。**先に `sql/006_figures.sql` を実行すること** |
| GET | `/me/garden` | 🔒 自分の箱庭の配置（模型をつかんで動かした場所）。`{"spots":[{"item_id","x","z"}]}`。まだ動かしていなければ空の配列 |
| PUT | `/me/garden` | 🔒 配置をまとめて保存（前の配置は消える。空の `spots` で並べ直し）。自分が出品した商品と買った商品だけ（ほかは 403）、200 個まで、位置は ±20 に直す、1分30回まで。**先に `sql/007_garden.sql` を実行すること** |
| GET | `/items/{id}` | 商品1件。無ければ 404。商品の `seller` は `{"id","display_name"}`（見本データは `null`）、`figure` は保存された模型の設計図（無ければ `null`。一覧・マイページも同じ） |
| POST | `/items/{id}/purchase` | 🔒 購入（決済なし）。201 と `{"purchase_id","item_id","price"}`。売り切れ・自分の商品は 409（`already_sold` / `own_item`）、無ければ 404。商品の行を `SELECT … FOR UPDATE` で押さえるので、同時に買われても1人だけ成功する |
| POST | `/items/{id}/conversations` | 🔒 この商品の出品者とのやりとりを始める。あれば 200 で既存、無ければ 201。自分の商品は 409 `own_item`、見本データ（出品者なし）は 409 `no_seller` |
| GET | `/conversations` | 🔒 自分のやりとり `[{"id","item","partner","last_message"}]`（最新メッセージが新しい順） |
| GET | `/conversations/{id}/messages` | 🔒 メッセージ（古い順）。`?after=<メッセージID>` でそれより新しいものだけ（画面は5秒ごとに呼ぶ）。参加者以外は 403 |
| POST | `/conversations/{id}/messages` | 🔒 `{"body":"..."}`（1〜1000文字）を送り 201 |
| GET | `/health` | DB に届けば `ok`（Cloud Run では `/healthz` は予約パスで使えない） |

## 環境変数

| 名前 | Cloud Run での値 | ローカルでの値 |
|---|---|---|
| `MYSQL_USER` | `uttc` | `uttc` |
| `MYSQL_PWD` | uttc ユーザーのパスワード | 同左 |
| `MYSQL_DATABASE` | `hackathon` | `hackathon` |
| `MYSQL_HOST` | `unix(/cloudsql/<接続名>)` | `tcp(127.0.0.1:3306)`（省略時もこれ） |
| `ALLOWED_ORIGINS` | `https://<あなたの>.vercel.app` | `http://localhost:5173` |
| `FIREBASE_PROJECT_ID` | 設定しない（省略時 `uttc-c10-yusuke-ota-5da712`） | 同左 |
| `GEMINI_MODEL` | 設定しない（省略時 `gemini-3.5-flash-lite`。2027-07 まで提供） | 同左 |
| `FIGURE_MODEL` | 設定しない（省略時 `gemini-4-argon`（2026-10 GA の最上位モデル）。写真→模型だけに使う。失敗したら `GEMINI_MODEL` でやり直す） | 同左 |
| `FIGURE_THINKING` | 設定しない（省略時 `medium`。`low`/`medium`/`high`。high は3分を超えることがある） | 同左 |
| `GOOGLE_CLOUD_LOCATION` | 設定しない（省略時 `global`） | 同左 |
| `PORT` | Cloud Run が自動で入れる（設定しない） | 省略時 8000 |

## Gemini を使うための設定（1回だけ）

- Vertex AI API を有効にする（Google Cloud コンソール →「Vertex AI API」→ 有効にする）
- Cloud Run のサービスアカウントに「Vertex AI ユーザー」（`roles/aiplatform.user`）を付ける
- ローカルではメタデータサーバーが無いので、説明文の生成は 502 になる（他の機能は動く）

## ローカルで動かす（PowerShell）

```powershell
$env:MYSQL_USER="uttc"; $env:MYSQL_PWD="..."; $env:MYSQL_DATABASE="hackathon"; $env:MYSQL_HOST="tcp(127.0.0.1:3306)"
go run .
go test ./...
```

## 悪用への歯止め（2026-11 のセキュリティ点検で追加）
- 本文の上限：JSON を受ける API は 256KB まで（`controller/limit.go`）。画像は 1.5MB、模型づくりは画像2枚ぶん
- 回数制限（1人あたり・サーバー1台のメモリ内）：出品 10回/分、メッセージ 30回/分、画像 10枚/分、説明文 5回/分、模型づくり 6回/分
- 画像は1人 24時間に 100 枚まで（DB で数える。超えたら 429 `image_quota`）
- 商品一覧は新しい順に 100 件まで
- 模型の「札」は1つの模型に6枚まで、幅 0.1 以上、高さ÷幅 0.1〜1
- 売れた後は、出品者も模型を上書きできない（購入者の箱庭を守る）
- Firebase の公開鍵は、知らない kid が来ても取り直しは1分に1回まで（取得は5秒で打ち切り、失敗したら古い鍵を使う）
- サーバーのタイムアウト：ヘッダー 10秒、本文 60秒、応答 300秒、待機 120秒
- 課題用の `/user`・`/users` は消した（だれでも書き込めたため。`user` テーブルは DB に残っている）
