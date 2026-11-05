package usecase

import (
	"time"

	"github.com/oklog/ulid/v2"

	"hackathon-backend/model"
)

// PurchaseStore は usecase が DB に求める操作。
type PurchaseStore interface {
	// Buy は1つのトランザクションで、商品を押さえて（SELECT … FOR UPDATE）確かめ、
	// purchases に入れて商品を sold にする。
	// 無ければ model.ErrNotFound、自分の商品なら model.ErrOwnItem、売り切れなら model.ErrAlreadySold。
	Buy(p model.Purchase) (model.Purchase, error)
	// FindByBuyer は買った記録を新しい順に返す。
	FindByBuyer(buyerId string) ([]model.Purchase, error)
}

type PurchaseUsecase struct {
	Store PurchaseStore
	Now   func() time.Time
}

// Buy は buyer が商品 itemId を買う。
func (uc *PurchaseUsecase) Buy(buyer model.Member, itemId string) (model.PurchaseRes, error) {
	if itemId == "" || buyer.Id == "" {
		return model.PurchaseRes{}, ErrInvalidInput
	}
	now := time.Now
	if uc.Now != nil {
		now = uc.Now
	}
	p, err := uc.Store.Buy(model.Purchase{
		Id:        ulid.Make().String(),
		Item:      model.Item{Id: itemId},
		BuyerId:   buyer.Id,
		CreatedAt: now().Truncate(time.Second),
	})
	if err != nil {
		return model.PurchaseRes{}, err
	}
	return model.PurchaseRes{PurchaseId: p.Id, ItemId: p.Item.Id, Price: p.Price}, nil
}

// History は buyer が買ったもの（新しい順）。
func (uc *PurchaseUsecase) History(buyer model.Member) ([]model.Purchase, error) {
	return uc.Store.FindByBuyer(buyer.Id)
}
