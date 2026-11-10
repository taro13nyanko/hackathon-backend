package usecase

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/oklog/ulid/v2"

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
	it, err := uc.Items.FindById(itemId)
	if err != nil {
		return model.Conversation{}, false, err
	}
	switch {
	case it.Seller == nil:
		return model.Conversation{}, false, model.ErrNoSeller
	case it.Seller.Id == me.Id:
		return model.Conversation{}, false, model.ErrOwnItem
	}
	id, created, err := uc.Store.FindOrCreate(model.Conversation{
		Id:        ulid.Make().String(),
		Item:      it,
		BuyerId:   me.Id,
		CreatedAt: uc.now().Truncate(time.Second),
	})
	if err != nil {
		return model.Conversation{}, false, err
	}
	c, err := uc.Store.Get(id, me.Id)
	return c, created, err
}

// List は自分が参加しているやりとり。
func (uc *DMUsecase) List(me model.Member) ([]model.Conversation, error) {
	return uc.Store.ListFor(me.Id)
}

// check は me がやりとり id の参加者かを確かめる。
func (uc *DMUsecase) check(me model.Member, id string) error {
	if id == "" {
		return ErrInvalidInput
	}
	buyer, seller, err := uc.Store.Participants(id)
	if err != nil {
		return err
	}
	if me.Id != buyer && me.Id != seller {
		return model.ErrForbidden
	}
	return nil
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
	m := model.Message{Id: ulid.Make().String(), SenderId: me.Id, Body: body, CreatedAt: uc.now().Truncate(time.Second)}
	if err := uc.Store.InsertMessage(id, m); err != nil {
		return model.Message{}, err
	}
	return m, nil
}
