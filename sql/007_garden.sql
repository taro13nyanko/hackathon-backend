-- 箱庭の配置（2026-11）。模型をつかんで動かした場所を、人ごと・商品ごとに覚える。
-- Cloud SQL Studio で hackathon データベースに対して実行する。backend をデプロイする前に実行すること
-- （先にデプロイしても壊れるのは GET/PUT /me/garden だけで、ほかの API は動く）。
CREATE TABLE IF NOT EXISTS garden_spots (
  user_id    CHAR(26) NOT NULL,           -- 箱庭の持ち主（users.id）
  item_id    CHAR(26) NOT NULL,           -- 置いた模型の商品（item.id）
  x          DOUBLE   NOT NULL,           -- 島の中心からの位置（左右）
  z          DOUBLE   NOT NULL,           -- 島の中心からの位置（前後）
  updated_at DATETIME NOT NULL,
  PRIMARY KEY (user_id, item_id),
  CONSTRAINT fk_garden_user FOREIGN KEY (user_id) REFERENCES users (id),
  CONSTRAINT fk_garden_item FOREIGN KEY (item_id) REFERENCES item (id)
) DEFAULT CHARSET = utf8mb4;
