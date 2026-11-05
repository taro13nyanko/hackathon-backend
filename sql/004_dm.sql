-- DM 機能（2026-11）。Cloud SQL Studio で hackathon データベースに対して1文ずつ実行する。

-- やりとり：1つの商品について、買いたい人1人ごとに1つ。相手（出品者）は item.seller_id から分かる。
CREATE TABLE IF NOT EXISTS conversations (
  id         CHAR(26) NOT NULL PRIMARY KEY,
  item_id    CHAR(26) NOT NULL,
  buyer_id   CHAR(26) NOT NULL,
  created_at DATETIME NOT NULL,
  UNIQUE KEY uq_conversations_item_buyer (item_id, buyer_id),
  INDEX idx_conversations_buyer (buyer_id),
  CONSTRAINT fk_conversations_item  FOREIGN KEY (item_id)  REFERENCES item (id),
  CONSTRAINT fk_conversations_buyer FOREIGN KEY (buyer_id) REFERENCES users (id)
) DEFAULT CHARSET = utf8mb4;

-- メッセージ：ID は ULID なので、ID の順＝送った順（?after= で新しいものだけ取れる）。
CREATE TABLE IF NOT EXISTS messages (
  id              CHAR(26)      NOT NULL PRIMARY KEY,
  conversation_id CHAR(26)      NOT NULL,
  sender_id       CHAR(26)      NOT NULL,
  body            VARCHAR(1000) NOT NULL,
  created_at      DATETIME      NOT NULL,
  INDEX idx_messages_conv (conversation_id, id),
  CONSTRAINT fk_messages_conv   FOREIGN KEY (conversation_id) REFERENCES conversations (id),
  CONSTRAINT fk_messages_sender FOREIGN KEY (sender_id)       REFERENCES users (id)
) DEFAULT CHARSET = utf8mb4;
