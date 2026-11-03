# hackathon-backend

UTTC ハッカソン用のバックエンド（Go）。Cloud Run で動かし、Cloud SQL（MySQL 8.0）に接続する。
カリキュラムの `db` ブランチのコードを、controller / usecase / dao / model の4層に分けたもの。

| パス | 中身 |
|---|---|
| `main.go` | 起動・ルーティング・終了処理（SIGTERM で処理中のリクエストを終えてから止まる） |
| `controller/` | HTTP の受け口（`/user`・`/users`・`/items`）と CORS |
| `usecase/` | 入力チェック（ユーザー・商品）と ID 採番（ULID） |
| `dao/` | SQL と DB 接続（接続情報はすべて環境変数） |
| `model/` | 構造体 |
| `sql/schema.sql` | `user`・`item` テーブルの CREATE 文と見本データ |

## API

本番: https://hackathon-backend-646017661953.us-central1.run.app

| メソッド | パス | 内容 |
|---|---|---|
| GET | `/items` | 商品一覧（新しい順）。`?q=キーワード` でタイトル・説明を部分一致検索。0件なら `[]` |
| POST | `/items` | `{"title":"...","description":"...","price":1200}` で出品し、201 と `{"id":"..."}` を返す。商品名1〜40文字・説明1000文字まで・価格300〜9,999,999円（違反は 400） |
| GET | `/items/{id}` | 商品1件。無ければ 404 |
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
| `PORT` | Cloud Run が自動で入れる（設定しない） | 省略時 8000 |

## ローカルで動かす（PowerShell）

```powershell
$env:MYSQL_USER="uttc"; $env:MYSQL_PWD="..."; $env:MYSQL_DATABASE="hackathon"; $env:MYSQL_HOST="tcp(127.0.0.1:3306)"
go run .
go test ./...
```
