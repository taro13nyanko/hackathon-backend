package dao

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"hackathon-backend/model"
)

type ItemDao struct {
	DB *sql.DB
}

// itemCols は商品の列（別名 i）。itemCols を SELECT したら、itemScan で受ける。
// 確認の理由（moderation_reason）はここに入れない：やりとり・購入履歴など、ほかの人に見せる所へ漏れないように、
// 理由を出してよい一覧・詳細（itemSelect）だけで読む。
const itemCols = "i.id, i.title, i.description, i.price, i.status, i.created_at, i.photo_image_id, i.figure_image_id, i.figure_caption, i.figure_json, i.moderation_status"

// itemScan は itemCols を受ける Scan 先と、受けた後に NULL を空文字にする関数を返す。
func itemScan(it *model.Item) ([]any, func()) {
	var photo, figure, figureJSON sql.NullString
	dest := []any{&it.Id, &it.Title, &it.Description, &it.Price, &it.Status, &it.CreatedAt, &photo, &figure, &it.FigureCaption, &figureJSON, &it.ModerationStatus}
	return dest, func() {
		it.PhotoImageId, it.FigureImageId = photo.String, figure.String
		if figureJSON.Valid && figureJSON.String != "" {
			it.Figure = json.RawMessage(figureJSON.String)
		}
	}
}

// itemSelect は商品と、出品者の表示名・確認の理由・いいねの数・本人がいいね済みかを1本の SQL で取る（N+1 にしない）。
//   - 出品者は users から LEFT JOIN（見本データは出品者なし → seller は null）。
//   - いいねの数は相関サブクエリ（idx_likes_item で商品ごとに数える）。
//   - liked は likes の主キー (user_id, item_id) を引く。最初の ? に本人の users.id を渡す（未ログインは空文字 → いつも false）。
//
// 理由（moderation_reason）は出品者本人と管理者にだけ見せる。消すのは usecase（redact）。
const itemSelect = "SELECT " + itemCols + ", u.id, u.display_name, i.moderation_reason," +
	" (SELECT COUNT(*) FROM likes l WHERE l.item_id = i.id)," +
	" EXISTS (SELECT 1 FROM likes l WHERE l.user_id = ? AND l.item_id = i.id)" +
	" FROM item i LEFT JOIN users u ON u.id = i.seller_id"

// itemPublic は一覧に出す商品（hidden を除く）。<> 'hidden' ではなく IN で書くと、
// idx_item_moderation_created (moderation_status, created_at) の範囲で読める。
const itemPublic = "i.moderation_status IN ('" + model.ModerationOK + "', '" + model.ModerationReview + "')"

type scanner interface{ Scan(dest ...any) error }

func scanItem(s scanner) (model.Item, error) {
	var it model.Item
	var sellerId, sellerName sql.NullString
	dest, done := itemScan(&it)
	if err := s.Scan(append(dest, &sellerId, &sellerName, &it.ModerationReason, &it.LikeCount, &it.Liked)...); err != nil {
		return model.Item{}, err
	}
	done()
	if sellerId.Valid {
		it.Seller = &model.Seller{Id: sellerId.String, DisplayName: sellerName.String}
	}
	return it, nil
}

// queryItems は itemSelect で始まる SQL を流して、商品の一覧を返す（0件なら空の配列）。
func (d *ItemDao) queryItems(query string, args ...any) ([]model.Item, error) {
	rows, err := d.DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("db.Query: %w", err)
	}
	defer rows.Close()
	items := make([]model.Item, 0)
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, fmt.Errorf("rows.Scan: %w", err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows.Err: %w", err)
	}
	return items, nil
}

// ItemListMax は一覧で返す件数の上限（新しい順に）。
const ItemListMax = 100

// FindAll は新しい順に商品を ItemListMax 件まで返す（hidden は除く）。
// q.Keyword が空でなければ、タイトルか説明文に含むものだけ。q.MinPrice・q.MaxPrice は値段の範囲（0 は指定なし）。
// liked は q.ViewerId の人として計算する。
func (d *ItemDao) FindAll(q model.ItemQuery) ([]model.Item, error) {
	query := itemSelect + " WHERE " + itemPublic
	args := []any{q.ViewerId}
	if q.Keyword != "" {
		// LIKE の特殊文字（% _ \）はエスケープして、文字どおりに探す
		like := "%" + escapeLike(q.Keyword) + "%"
		query += " AND (i.title LIKE ? OR i.description LIKE ?)"
		args = append(args, like, like)
	}
	// 値段の範囲（音声検索の「3000円以下」など）。0 は指定なし
	if q.MinPrice > 0 {
		query += " AND i.price >= ?"
		args = append(args, q.MinPrice)
	}
	if q.MaxPrice > 0 {
		query += " AND i.price <= ?"
		args = append(args, q.MaxPrice)
	}
	query += " ORDER BY i.created_at DESC, i.id DESC LIMIT ?"
	args = append(args, ItemListMax)
	return d.queryItems(query, args...)
}

// FindBySeller は出品者 sellerId の商品を新しい順に返す（マイページ用。本人なので hidden も含める）。
func (d *ItemDao) FindBySeller(sellerId string) ([]model.Item, error) {
	return d.queryItems(itemSelect+" WHERE i.seller_id = ? ORDER BY i.created_at DESC, i.id DESC", sellerId, sellerId)
}

// FindById は1件返す（hidden も返す。だれに見せるかは usecase が決める）。無ければ model.ErrNotFound。
func (d *ItemDao) FindById(id, viewerId string) (model.Item, error) {
	it, err := scanItem(d.DB.QueryRow(itemSelect+" WHERE i.id = ?", viewerId, id))
	if errors.Is(err, sql.ErrNoRows) {
		return model.Item{}, model.ErrNotFound
	}
	if err != nil {
		return model.Item{}, fmt.Errorf("db.QueryRow: %w", err)
	}
	return it, nil
}

// ModerationListMax は管理画面で返す件数の上限（新しい順に）。
const ModerationListMax = 100

// FindByModeration は確認の状態が status の商品を新しい順に返す（管理画面用。idx_item_moderation_created を使う）。
func (d *ItemDao) FindByModeration(status, viewerId string) ([]model.Item, error) {
	return d.queryItems(itemSelect+" WHERE i.moderation_status = ? ORDER BY i.created_at DESC, i.id DESC LIMIT ?",
		viewerId, status, ModerationListMax)
}

// SetModeration は商品の確認の状態を変える。reason が nil なら理由はそのまま。商品が無ければ model.ErrNotFound。
func (d *ItemDao) SetModeration(id, status string, reason *string) error {
	q, args := "UPDATE item SET moderation_status = ? WHERE id = ?", []any{status, id}
	if reason != nil {
		q, args = "UPDATE item SET moderation_status = ?, moderation_reason = ? WHERE id = ?", []any{status, *reason, id}
	}
	if _, err := d.DB.Exec(q, args...); err != nil {
		return fmt.Errorf("db.Exec: %w", err)
	}
	// MySQL は前と同じ値への UPDATE を 0 行と数えるので、行数ではなく商品があるかで確かめる
	var one int
	err := d.DB.QueryRow("SELECT 1 FROM item WHERE id = ?", id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return model.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("db.QueryRow: %w", err)
	}
	return nil
}

// Insert はトランザクションで1件登録する（失敗したら Rollback）。出品者は必須。
func (d *ItemDao) Insert(it model.Item) error {
	if it.Seller == nil {
		return errors.New("item.Insert: 出品者がない")
	}
	tx, err := d.DB.Begin()
	if err != nil {
		return fmt.Errorf("db.Begin: %w", err)
	}
	_, err = tx.Exec(
		"INSERT INTO item (id, title, description, price, status, moderation_status, moderation_reason, seller_id, photo_image_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		it.Id, it.Title, it.Description, it.Price, it.Status, it.ModerationStatus, it.ModerationReason, it.Seller.Id, nullIfEmpty(it.PhotoImageId), it.CreatedAt,
	)
	if err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("tx.Exec: %w (rollback: %v)", err, rbErr)
		}
		return fmt.Errorf("tx.Exec: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("tx.Commit: %w", err)
	}
	return nil
}

// SaveFigure は商品に模型の設計図（JSON）と一言を保存する。
// onlyIfEmpty なら、まだ模型が無いときだけ保存する（購入者用。出品者の模型を上書きしない）。
// 保存しなかった（商品が無い・既に模型がある）ときは false。
// ※ MySQL は前と同じ値への UPDATE を 0 行と数えるので、onlyIfEmpty でないときの false は「同じ模型を保存し直した」こともある。
func (d *ItemDao) SaveFigure(id, figureJSON, caption string, onlyIfEmpty bool) (bool, error) {
	q := "UPDATE item SET figure_json = ?, figure_caption = ? WHERE id = ?"
	if onlyIfEmpty {
		q += " AND figure_json IS NULL"
	}
	res, err := d.DB.Exec(q, figureJSON, caption, id)
	if err != nil {
		return false, fmt.Errorf("db.Exec: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("RowsAffected: %w", err)
	}
	return n == 1, nil
}

// BuyerOf はその商品を買った人の ID を返す。まだ誰も買っていなければ空文字。
func (d *ItemDao) BuyerOf(itemId string) (string, error) {
	var buyer string
	err := d.DB.QueryRow("SELECT buyer_id FROM purchases WHERE item_id = ?", itemId).Scan(&buyer)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("db.QueryRow: %w", err)
	}
	return buyer, nil
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// nullIfEmpty は空文字を NULL にする（外部キーの列に ” を入れないため）。
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
