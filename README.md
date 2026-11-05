# hackathon-backend

UTTC ハッカソン用のバックエンド（Go）。Cloud Run で動かし、Cloud SQL（MySQL 8.0）に接続する。
カリキュラムの `db` ブランチのコードを、controller / usecase / dao / model の4層に分けたもの。

| パス | 中身 |
|---|---|
| `main.go` | 起動・ルーティング・終了処理（SIGTERM で処理中のリクエストを終えてから止まる） |
| `auth/` | Firebase の ID トークンの確認（署名・期限・宛先。標準ライブラリだけ） |
| `controller/` | HTTP の受け口（`/me`・`/items`・`/user`・`/users`）、ログイン確認、CORS |
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
エラーは `{"error":"<コード>"}`（`invalid_input` `unauthorized` `profile_required` `not_found` `already_registered` `already_sold` `own_item` `no_seller` `forbidden` `internal`）。

| メソッド | パス | 内容 |
|---|---|---|
| GET | `/me` | 🔒 自分の `{"id","display_name"}`。表示名が未登録なら 404（画面はプロフィール登録へ） |
| POST | `/me` | 🔒 `{"display_name":"ota"}`（1〜50文字）で登録し 201。登録済みなら 409 |
| GET | `/me/items` | 🔒 自分が出品した商品（新しい順） |
| GET | `/me/purchases` | 🔒 自分が買った商品 `[{"id","item","price","created_at"}]`（新しい順） |
| GET | `/items` | 商品一覧（新しい順）。`?q=キーワード` でタイトル・説明を部分一致検索。0件なら `[]` |
| POST | `/items` | 🔒 表示名の登録が必要（未登録は 403 `profile_required`）。`{"title":"...","description":"...","price":1200}` で出品し、201 と `{"id":"..."}` を返す。商品名1〜40文字・説明1000文字まで・価格300〜9,999,999円（違反は 400） |
| GET | `/items/{id}` | 商品1件。無ければ 404。商品の `seller` は `{"id","display_name"}`（見本データは `null`） |
| POST | `/items/{id}/purchase` | 🔒 購入（決済なし）。201 と `{"purchase_id","item_id","price"}`。売り切れ・自分の商品は 409（`already_sold` / `own_item`）、無ければ 404。商品の行を `SELECT … FOR UPDATE` で押さえるので、同時に買われても1人だけ成功する |
| POST | `/items/{id}/conversations` | 🔒 この商品の出品者とのやりとりを始める。あれば 200 で既存、無ければ 201。自分の商品は 409 `own_item`、見本データ（出品者なし）は 409 `no_seller` |
| GET | `/conversations` | 🔒 自分のやりとり `[{"id","item","partner","last_message"}]`（最新メッセージが新しい順） |
| GET | `/conversations/{id}/messages` | 🔒 メッセージ（古い順）。`?after=<メッセージID>` でそれより新しいものだけ（画面は5秒ごとに呼ぶ）。参加者以外は 403 |
| POST | `/conversations/{id}/messages` | 🔒 `{"body":"..."}`（1〜1000文字）を送り 201 |
| GET | `/users` | 全ユーザー |
| GET | `/user?name=hanako` | 名前が一致するユーザーの配列（0件なら `[]`） |
| POST | `/user` | `{"name":"hanako","age":20}` を登録し `{"id":"..."}` を返す |
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
| `PORT` | Cloud Run が自動で入れる（設定しない） | 省略時 8000 |

## ローカルで動かす（PowerShell）

```powershell
$env:MYSQL_USER="uttc"; $env:MYSQL_PWD="..."; $env:MYSQL_DATABASE="hackathon"; $env:MYSQL_HOST="tcp(127.0.0.1:3306)"
go run .
go test ./...
```
