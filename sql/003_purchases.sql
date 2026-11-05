-- 購入機能（2026-11）。Cloud SQL Studio で hackathon データベースに対して実行する。
-- 決済はしないので、買った記録だけを残す。1つの商品は1回しか買えない（item_id が UNIQUE）。
CREATE TABLE IF NOT EXISTS purchases (
  id         CHAR(26) NOT NULL PRIMARY KEY,  -- ULID
  item_id    CHAR(26) NOT NULL UNIQUE,
  buyer_id   CHAR(26) NOT NULL,
  price      INT      NOT NULL,              -- 買ったときの価格
  created_at DATETIME NOT NULL,
  INDEX idx_purchases_buyer (buyer_id, created_at),
  CONSTRAINT fk_purchases_item  FOREIGN KEY (item_id)  REFERENCES item (id),
  CONSTRAINT fk_purchases_buyer FOREIGN KEY (buyer_id) REFERENCES users (id)
) DEFAULT CHARSET = utf8mb4;
