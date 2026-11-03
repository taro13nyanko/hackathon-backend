-- Cloud SQL の hackathon データベースで実行する（GoLand のクエリコンソールに貼って全選択→実行）
CREATE TABLE IF NOT EXISTS user (
  id   CHAR(26)    NOT NULL PRIMARY KEY,  -- ULID は26文字
  name VARCHAR(50) NOT NULL,
  age  INT         NOT NULL
);

-- 動作確認用の1件（不要なら消してよい）
INSERT INTO user (id, name, age) VALUES ('00000000000000000000000000', 'hanako', 20);

-- 出品された商品（段階3）。seller_id はログイン機能を入れた段階で追加する。
CREATE TABLE IF NOT EXISTS item (
  id          CHAR(26)      NOT NULL PRIMARY KEY,           -- ULID
  title       VARCHAR(40)   NOT NULL,
  description VARCHAR(1000) NOT NULL DEFAULT '',
  price       INT           NOT NULL,                       -- 円。300〜9,999,999（usecase で確認）
  status      VARCHAR(16)   NOT NULL DEFAULT 'on_sale',     -- on_sale / sold
  created_at  DATETIME      NOT NULL,
  INDEX idx_item_created_at (created_at)
) DEFAULT CHARSET = utf8mb4;

-- 動作確認用の商品（不要なら消してよい）
INSERT INTO item (id, title, description, price, status, created_at) VALUES
  ('01JBX0000000000000000000A1', '無印良品のスタッキングシェルフ', '1年使用。目立つ傷なし。手渡し希望（駒場・本郷）。', 4500, 'on_sale', '2026-11-01 10:00:00'),
  ('01JBX0000000000000000000A2', '線形代数の教科書（東大出版会）', '書き込み少しあり。', 1200, 'on_sale', '2026-11-01 10:05:00');
