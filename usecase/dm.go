package usecase

import (
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"hackathon-backend/model"
)

// MessageBodyMax はメッセージ1通の最大文字数。
const MessageBodyMax = 1000

// DMStore は usecase が DB に求める操作。
type DMStore interface {
	// FindOrCreate は (itemId, buyerId) のやりとりを返す。無ければ c で作る。作ったら created=true。
	FindOrCreate(c model.Conversation) (id string, created bool, err error)
	// Get は viewerId から見たやりとり1件（相手・最新メッセージつき）。無ければ model.ErrNotFound。
	Get(id, viewerId string) (model.Conversation, error)
	// ListFor は viewerId が参加しているやりとり（最新メッセージが新しい順）。
	ListFor(viewerId string) ([]model.Conversation, error)
	// Participants はやりとりの2人（買い手と出品者）。無ければ model.ErrNotFound。
	Participants(id string) (buyerId, sellerId string, err error)
	// Messages は afterId より新しいメッセージを古い順に返す（afterId が空なら全部）。
	Messages(conversationId, afterId string) ([]model.Message, error)
	InsertMessage(conversationId string, m model.Message) error
	// UnreadCounts は viewerId の未読の数（やりとりの ID → 数）。未読が0のやりとりは入らない。
	// 未読＝相手が送った、既読の位置より新しいメッセージ。1本の SQL で全部のやりとりを数える。
	UnreadCounts(viewerId string) (map[string]int, error)
	// MarkRead は userId の既読の位置を、やりとり内の upTo 以下で最新のメッセージまで進める（upTo が空なら最新まで）。
	// 後戻りはしない。進めた後の既読の位置を返す（メッセージが無ければ空）。
	MarkRead(conversationId, userId, upTo string, at time.Time) (string, error)
	// LastRead は userId の既読の位置（やりとり conversationId の中で、どのメッセージまで読んだか）。まだ無ければ空。
	LastRead(conversationId, userId string) (string, error)
}

type DMUsecase struct {
	Store DMStore
	Items ItemStore
	Limit *Limiter // メッセージ送信の回数制限（nil なら無し）
	Now   func() time.Time
}

func (uc *DMUsecase) now() time.Time {
	if uc.Now != nil {
		return uc.Now()
	}
	return time.Now()
}

// Start は商品 itemId の出品者とのやりとりを始める（もうあれば、それを返す）。
func (uc *DMUsecase) Start(me model.Member, itemId string) (model.Conversation, bool, error) {
	if itemId == "" {
		return model.Conversation{}, false, ErrInvalidInput
	}
	it, err := findActive(uc.Items, itemId, me.Id) // 非公開（hidden）の商品とはやりとりを始められない（404）
	if err != nil {
		return model.Conversation{}, false, err
	}
	switch {
	case it.Seller == nil:
		return model.Conversation{}, false, model.ErrNoSeller
	case it.Seller.Id == me.Id:
		return model.Conversation{}, false, model.ErrOwnItem
	}
	id, created, err := uc.Store.FindOrCreate(model.NewConversation(it, me.Id, uc.now()))
	if err != nil {
		return model.Conversation{}, false, err
	}
	c, err := uc.Store.Get(id, me.Id)
	return c, created, err
}

// List は自分が参加しているやりとり（それぞれ未読の数つき）。
// 未読が数えられなくても一覧は返す（unread_count は 0）。sql/009 を流す前に backend を出しても、壊れるのは未読だけにするため。
func (uc *DMUsecase) List(me model.Member) ([]model.Conversation, error) {
	cs, err := uc.Store.ListFor(me.Id)
	if err != nil {
		return nil, err
	}
	counts, err := uc.Store.UnreadCounts(me.Id)
	if err != nil {
		log.Printf("fail: unread counts, %v", err)
		return cs, nil
	}
	for i := range cs {
		cs[i].UnreadCount = counts[cs[i].Id]
	}
	return cs, nil
}

// Unread は自分の未読の合計（ナビの 💬 に出す数）。
func (uc *DMUsecase) Unread(me model.Member) (model.UnreadRes, error) {
	counts, err := uc.Store.UnreadCounts(me.Id)
	if err != nil {
		return model.UnreadRes{}, err
	}
	n := 0
	for _, c := range counts {
		n += c
	}
	return model.UnreadRes{Count: n}, nil
}

// MarkRead は会話を開いたとき・新しいメッセージを画面に出したときに呼ぶ。
// req.LastMessageId（画面に出した最後のメッセージ）までを既読にする。空なら最新まで。
// 既読の位置は後戻りしない（古い ID を渡しても、それより先に読んだ分は既読のまま）。参加者以外は 403。
func (uc *DMUsecase) MarkRead(me model.Member, id string, req model.ReadReq) (model.ReadRes, error) {
	upTo := strings.TrimSpace(req.LastMessageId)
	if len(upTo) > 26 { // メッセージの ID は ULID（26文字）
		return model.ReadRes{}, ErrInvalidInput
	}
	if err := uc.check(me, id); err != nil {
		return model.ReadRes{}, err
	}
	last, err := uc.Store.MarkRead(id, me.Id, upTo, uc.now())
	if err != nil {
		return model.ReadRes{}, err
	}
	total, err := uc.Unread(me)
	if err != nil {
		return model.ReadRes{}, err
	}
	return model.ReadRes{LastReadMessageId: last, UnreadCount: total.Count}, nil
}

// PeerRead は相手（自分が買い手なら出品者、出品者なら買い手）がどのメッセージまで読んだか（画面の「既読」用）。
// 自分が送ったメッセージのうち、ID が返した位置以下のものに「既読」を付ける。参加者以外は 403。
func (uc *DMUsecase) PeerRead(me model.Member, id string) (model.PeerReadRes, error) {
	buyer, seller, err := uc.participants(me, id)
	if err != nil {
		return model.PeerReadRes{}, err
	}
	peer := seller
	if me.Id == seller {
		peer = buyer
	}
	if peer == "" { // 出品者がいない（見本データ）
		return model.PeerReadRes{}, nil
	}
	last, err := uc.Store.LastRead(id, peer)
	if err != nil {
		return model.PeerReadRes{}, err
	}
	return model.PeerReadRes{PeerLastReadMessageId: last}, nil
}

// check は me がやりとり id の参加者かを確かめる。
func (uc *DMUsecase) check(me model.Member, id string) error {
	_, _, err := uc.participants(me, id)
	return err
}

// participants は me がやりとり id の参加者かを確かめ、2人（買い手と出品者）を返す。
func (uc *DMUsecase) participants(me model.Member, id string) (buyer, seller string, err error) {
	if id == "" {
		return "", "", ErrInvalidInput
	}
	buyer, seller, err = uc.Store.Participants(id)
	if err != nil {
		return "", "", err
	}
	if me.Id != buyer && me.Id != seller {
		return "", "", model.ErrForbidden
	}
	return buyer, seller, nil
}

// Messages は afterId より新しいメッセージ（画面は5秒ごとに呼ぶ）。
func (uc *DMUsecase) Messages(me model.Member, id, afterId string) ([]model.Message, error) {
	if err := uc.check(me, id); err != nil {
		return nil, err
	}
	return uc.Store.Messages(id, afterId)
}

// Send はメッセージを送る（1〜1000文字。前後の空白は除く）。
func (uc *DMUsecase) Send(me model.Member, id string, req model.MessageCreateReq) (model.Message, error) {
	body := strings.TrimSpace(req.Body)
	if body == "" || utf8.RuneCountInString(body) > MessageBodyMax {
		return model.Message{}, ErrInvalidInput
	}
	if err := uc.check(me, id); err != nil {
		return model.Message{}, err
	}
	if !uc.Limit.Allow(me.Id) {
		return model.Message{}, ErrTooMany
	}
	m := model.NewMessage(me.Id, body, uc.now())
	if err := uc.Store.InsertMessage(id, m); err != nil {
		return model.Message{}, err
	}
	return m, nil
}
