package dao

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"hackathon-backend/model"
	"hackathon-backend/usecase"
)

type DMDao struct {
	DB *sql.DB
}

var _ usecase.DMStore = (*DMDao)(nil)

// FindOrCreate は (item_id, buyer_id) のやりとりを探し、無ければ作る。
// 同時に2回作ろうとしても UNIQUE(item_id, buyer_id) で1つになる（負けた方は探し直す）。
func (d *DMDao) FindOrCreate(c model.Conversation) (string, bool, error) {
	find := func() (string, error) {
		var id string
		err := d.DB.QueryRow("SELECT id FROM conversations WHERE item_id = ? AND buyer_id = ?", c.Item.Id, c.BuyerId).Scan(&id)
		return id, err
	}
	id, err := find()
	if err == nil {
		return id, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", false, fmt.Errorf("select conversation: %w", err)
	}
	_, err = d.DB.Exec("INSERT INTO conversations (id, item_id, buyer_id, created_at) VALUES (?, ?, ?, ?)",
		c.Id, c.Item.Id, c.BuyerId, c.CreatedAt)
	if isDuplicate(err) {
		id, err := find()
		if err != nil {
			return "", false, fmt.Errorf("select conversation: %w", err)
		}
		return id, false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("insert conversation: %w", err)
	}
	return c.Id, true, nil
}

// やりとりと、商品・出品者・買い手・最新メッセージを1回で取る。
const convSelect = "SELECT c.id, c.buyer_id, c.created_at," +
	" " + itemCols + "," +
	" s.id, s.display_name, b.display_name," +
	" m.id, m.sender_id, m.body, m.created_at" +
	" FROM conversations c" +
	" JOIN item i ON i.id = c.item_id" +
	" JOIN users s ON s.id = i.seller_id" +
	" JOIN users b ON b.id = c.buyer_id" +
	" LEFT JOIN messages m ON m.id = (SELECT MAX(id) FROM messages WHERE conversation_id = c.id)"

func scanConv(sc scanner, viewerId string) (model.Conversation, error) {
	var c model.Conversation
	var sellerId, sellerName, buyerName string
	var mId, mSender, mBody sql.NullString
	var mAt sql.NullTime
	dest, done := itemScan(&c.Item)
	all := append([]any{&c.Id, &c.BuyerId, &c.CreatedAt}, dest...)
	if err := sc.Scan(append(all, &sellerId, &sellerName, &buyerName, &mId, &mSender, &mBody, &mAt)...); err != nil {
		return model.Conversation{}, err
	}
	done()
	c.Item.Seller = &model.Seller{Id: sellerId, DisplayName: sellerName}
	if viewerId == c.BuyerId {
		c.Partner = model.Seller{Id: sellerId, DisplayName: sellerName}
	} else {
		c.Partner = model.Seller{Id: c.BuyerId, DisplayName: buyerName}
	}
	if mId.Valid {
		c.LastMessage = &model.Message{Id: mId.String, SenderId: mSender.String, Body: mBody.String, CreatedAt: mAt.Time}
	}
	return c, nil
}

func (d *DMDao) Get(id, viewerId string) (model.Conversation, error) {
	c, err := scanConv(d.DB.QueryRow(convSelect+" WHERE c.id = ?", id), viewerId)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Conversation{}, model.ErrNotFound
	}
	if err != nil {
		return model.Conversation{}, fmt.Errorf("select conversation: %w", err)
	}
	return c, nil
}

func (d *DMDao) ListFor(viewerId string) ([]model.Conversation, error) {
	rows, err := d.DB.Query(convSelect+" WHERE c.buyer_id = ? OR i.seller_id = ?"+
		" ORDER BY COALESCE(m.created_at, c.created_at) DESC, c.id DESC", viewerId, viewerId)
	if err != nil {
		return nil, fmt.Errorf("db.Query: %w", err)
	}
	defer rows.Close()
	out := make([]model.Conversation, 0)
	for rows.Next() {
		c, err := scanConv(rows, viewerId)
		if err != nil {
			return nil, fmt.Errorf("rows.Scan: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (d *DMDao) Participants(id string) (string, string, error) {
	var buyer string
	var seller sql.NullString
	err := d.DB.QueryRow("SELECT c.buyer_id, i.seller_id FROM conversations c JOIN item i ON i.id = c.item_id WHERE c.id = ?", id).
		Scan(&buyer, &seller)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", model.ErrNotFound
	}
	if err != nil {
		return "", "", fmt.Errorf("select participants: %w", err)
	}
	return buyer, seller.String, nil
}

// Messages は afterId より新しいものを古い順に（1回に最大200件）。
func (d *DMDao) Messages(conversationId, afterId string) ([]model.Message, error) {
	rows, err := d.DB.Query("SELECT id, sender_id, body, created_at FROM messages"+
		" WHERE conversation_id = ? AND id > ? ORDER BY id LIMIT 200", conversationId, afterId)
	if err != nil {
		return nil, fmt.Errorf("db.Query: %w", err)
	}
	defer rows.Close()
	out := make([]model.Message, 0)
	for rows.Next() {
		var m model.Message
		if err := rows.Scan(&m.Id, &m.SenderId, &m.Body, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("rows.Scan: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (d *DMDao) InsertMessage(conversationId string, m model.Message) error {
	_, err := d.DB.Exec("INSERT INTO messages (id, conversation_id, sender_id, body, created_at) VALUES (?, ?, ?, ?, ?)",
		m.Id, conversationId, m.SenderId, m.Body, m.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert message: %w", err)
	}
	return nil
}

// UnreadCounts は viewerId が参加しているやりとりごとの未読の数（conversation_reads テーブル、sql/009_conversation_reads.sql）。
// 相手が送ったメッセージのうち、既読の位置より ID が大きいもの（ULID なので ID の順＝送った順）を、1本の SQL でまとめて数える。
// 既読の行がまだ無いやりとりは、相手のメッセージが全部未読。
func (d *DMDao) UnreadCounts(viewerId string) (map[string]int, error) {
	rows, err := d.DB.Query("SELECT m.conversation_id, COUNT(*)"+
		" FROM conversations c"+
		" JOIN item i ON i.id = c.item_id"+
		" JOIN messages m ON m.conversation_id = c.id"+
		" LEFT JOIN conversation_reads r ON r.conversation_id = c.id AND r.user_id = ?"+
		" WHERE (c.buyer_id = ? OR i.seller_id = ?)"+
		" AND m.sender_id <> ?"+
		" AND m.id > COALESCE(r.last_read_message_id, '')"+
		" GROUP BY m.conversation_id", viewerId, viewerId, viewerId, viewerId)
	if err != nil {
		return nil, fmt.Errorf("db.Query: %w", err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, fmt.Errorf("rows.Scan: %w", err)
		}
		out[id] = n
	}
	return out, rows.Err()
}

// MarkRead は既読の位置を、やりとり内の upTo 以下で最新のメッセージまで進める（upTo が空なら最新まで）。
// 実在するメッセージの ID しか入れない（でたらめな ID で先の分まで既読にはできない）。
// 既にもっと先まで読んでいれば GREATEST でそのまま（後戻りしない）。返り値は進めた後の既読の位置。
func (d *DMDao) MarkRead(conversationId, userId, upTo string, at time.Time) (string, error) {
	q := "SELECT MAX(id) FROM messages WHERE conversation_id = ?"
	args := []any{conversationId}
	if upTo != "" {
		q += " AND id <= ?"
		args = append(args, upTo)
	}
	var target sql.NullString
	if err := d.DB.QueryRow(q, args...).Scan(&target); err != nil {
		return "", fmt.Errorf("select last message: %w", err)
	}
	if target.Valid {
		_, err := d.DB.Exec("INSERT INTO conversation_reads (user_id, conversation_id, last_read_message_id, updated_at)"+
			" VALUES (?, ?, ?, ?)"+
			" ON DUPLICATE KEY UPDATE last_read_message_id = GREATEST(last_read_message_id, VALUES(last_read_message_id)),"+
			" updated_at = VALUES(updated_at)",
			userId, conversationId, target.String, at)
		if err != nil {
			return "", fmt.Errorf("upsert read: %w", err)
		}
	}
	var last string
	err := d.DB.QueryRow("SELECT last_read_message_id FROM conversation_reads WHERE user_id = ? AND conversation_id = ?",
		userId, conversationId).Scan(&last)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("select read: %w", err)
	}
	return last, nil
}
