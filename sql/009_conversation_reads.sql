-- 009：DM の既読（未読の数を出すため。2026-11-15）
-- 本番の Cloud SQL では backend を出す前に流すこと。
-- （先に backend を出しても壊れるのは未読だけ：一覧の unread_count が 0 のまま、PUT /conversations/{id}/read と GET /me/unread が 500。
--   一覧・メッセージの読み書きはこれまでどおり動く）
-- Cloud SQL Studio で hackathon データベースに対して1文ずつ実行する。

-- 既読の位置：人ごと・やりとりごとに「どのメッセージまで読んだか」。
-- メッセージの ID は ULID なので、last_read_message_id より大きい ID＝まだ読んでいないメッセージ。
-- 後戻りはさせない（dao/dm.go の MarkRead は GREATEST で大きい方を残す）。
CREATE TABLE IF NOT EXISTS conversation_reads (
  user_id              CHAR(26) NOT NULL,   -- 読んだ人（users.id。買い手か出品者）
  conversation_id      CHAR(26) NOT NULL,   -- やりとり（conversations.id）
  last_read_message_id CHAR(26) NOT NULL,   -- ここまで読んだ（messages.id）
  updated_at           DATETIME NOT NULL,
  PRIMARY KEY (user_id, conversation_id),
  INDEX idx_reads_conv (conversation_id),
  CONSTRAINT fk_reads_user FOREIGN KEY (user_id)         REFERENCES users (id),
  CONSTRAINT fk_reads_conv FOREIGN KEY (conversation_id) REFERENCES conversations (id)
) DEFAULT CHARSET = utf8mb4;

-- 今あるメッセージは読んだことにする（流した直後に、過去のメッセージが全部「未読」にならないように）。
-- 買い手の分と出品者の分。何度流しても同じ結果になる（INSERT IGNORE）。
INSERT IGNORE INTO conversation_reads (user_id, conversation_id, last_read_message_id, updated_at)
SELECT c.buyer_id, c.id, MAX(m.id), NOW()
FROM conversations c JOIN messages m ON m.conversation_id = c.id
GROUP BY c.id, c.buyer_id;

INSERT IGNORE INTO conversation_reads (user_id, conversation_id, last_read_message_id, updated_at)
SELECT i.seller_id, c.id, MAX(m.id), NOW()
FROM conversations c JOIN item i ON i.id = c.item_id JOIN messages m ON m.conversation_id = c.id
WHERE i.seller_id IS NOT NULL
GROUP BY c.id, i.seller_id;
