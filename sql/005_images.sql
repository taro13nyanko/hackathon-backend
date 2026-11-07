-- 写真と模型の絵（2026-11）。Cloud SQL Studio で hackathon データベースに対して1文ずつ実行する。
-- 画像は Cloud Storage を使わず DB に入れる（画面で 768px の JPEG に縮めるので 1枚 100KB 前後。MEDIUMBLOB は 16MB まで）。

CREATE TABLE IF NOT EXISTS images (
  id           CHAR(26)    NOT NULL PRIMARY KEY,
  owner_id     CHAR(26)    NOT NULL,              -- 上げた人
  content_type VARCHAR(32) NOT NULL,              -- image/jpeg・image/png・image/webp
  data         MEDIUMBLOB  NOT NULL,
  created_at   DATETIME    NOT NULL,
  INDEX idx_images_owner (owner_id, created_at),
  CONSTRAINT fk_images_owner FOREIGN KEY (owner_id) REFERENCES users (id)
) DEFAULT CHARSET = utf8mb4;

-- 商品に写真と模型（模型は次の段階で使う）。見本データは NULL のまま。
ALTER TABLE item
  ADD COLUMN photo_image_id  CHAR(26)     NULL AFTER seller_id,
  ADD COLUMN figure_image_id CHAR(26)     NULL AFTER photo_image_id,
  ADD COLUMN figure_caption  VARCHAR(200) NOT NULL DEFAULT '' AFTER figure_image_id,
  ADD CONSTRAINT fk_item_photo  FOREIGN KEY (photo_image_id)  REFERENCES images (id),
  ADD CONSTRAINT fk_item_figure FOREIGN KEY (figure_image_id) REFERENCES images (id);
