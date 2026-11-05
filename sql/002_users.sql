-- ログイン機能（2026-11）。Cloud SQL Studio で hackathon データベースに対して1文ずつ実行する。
-- 購入・DM のテーブルは、その機能を作るときに 003 以降で足す。

-- アプリの利用者（Google でログインした人）。カリキュラムの user テーブルとは別。
CREATE TABLE IF NOT EXISTS users (
  id           CHAR(26)     NOT NULL PRIMARY KEY,  -- ULID
  firebase_uid VARCHAR(128) NOT NULL UNIQUE,       -- Firebase のユーザー ID
  display_name VARCHAR(50)  NOT NULL,
  created_at   DATETIME     NOT NULL
) DEFAULT CHARSET = utf8mb4;

-- 商品に出品者を付ける。見本データの2件は NULL のまま（画面では「出品者なし」）。
ALTER TABLE item
  ADD COLUMN seller_id CHAR(26) NULL AFTER status,
  ADD INDEX idx_item_seller (seller_id),
  ADD CONSTRAINT fk_item_seller FOREIGN KEY (seller_id) REFERENCES users (id);
