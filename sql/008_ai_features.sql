-- 008：いいね・出品の確認（禁止物の検知）・ロール・一覧用のインデックス（2026-11-15）
-- 本番の Cloud SQL では backend を出す前に流すこと（先に出すと一覧と出品が 500 になる）
-- Cloud SQL Studio で hackathon データベースに対して1文ずつ実行する。

-- いいね：1人が1つの商品に1回だけ（主キーで重複を防ぐ）。
--   idx_likes_item         … 商品ごとのいいね数（一覧・詳細の like_count）
--   idx_likes_user_created … 自分のいいね一覧（GET /me/likes、いいねした新しい順）
CREATE TABLE likes (
  user_id    CHAR(26) NOT NULL,
  item_id    CHAR(26) NOT NULL,
  created_at DATETIME NOT NULL,
  PRIMARY KEY (user_id, item_id),
  INDEX idx_likes_item (item_id),
  INDEX idx_likes_user_created (user_id, created_at),
  CONSTRAINT fk_likes_user FOREIGN KEY (user_id) REFERENCES users(id),
  CONSTRAINT fk_likes_item FOREIGN KEY (item_id) REFERENCES item(id)
) DEFAULT CHARSET=utf8mb4;

-- 出品の確認：ok（問題なし）／review（要確認・公開はされる）／hidden（管理者が非公開にした）。
-- 既存の行は 'ok'（見本データ・既存の出品は確認済み扱い）。
--   idx_item_moderation_created … 一覧（hidden を除いて新しい順）と管理画面（review／hidden を新しい順）
ALTER TABLE item
  ADD COLUMN moderation_status VARCHAR(16) NOT NULL DEFAULT 'ok' AFTER status,
  ADD COLUMN moderation_reason VARCHAR(300) NOT NULL DEFAULT '' AFTER moderation_status,
  ADD INDEX idx_item_moderation_created (moderation_status, created_at);

-- ロール：member（既定）／admin（管理者）。
ALTER TABLE users
  ADD COLUMN role VARCHAR(16) NOT NULL DEFAULT 'member' AFTER display_name;
-- 管理者にする（本人が自分の users.id で実行）：UPDATE users SET role='admin' WHERE id='...';
