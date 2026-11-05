package usecase

import (
	"errors"
	"testing"

	"hackathon-backend/model"
)

type fakePurchaseStore struct {
	got model.Purchase
	err error
}

func (f *fakePurchaseStore) Buy(p model.Purchase) (model.Purchase, error) {
	f.got = p
	p.Price = 1200
	return p, f.err
}
func (f *fakePurchaseStore) FindByBuyer(string) ([]model.Purchase, error) { return nil, nil }

func TestBuy(t *testing.T) {
	store := &fakePurchaseStore{}
	uc := &PurchaseUsecase{Store: store}
	res, err := uc.Buy(testSeller, "ITEM1")
	if err != nil || res.ItemId != "ITEM1" || res.Price != 1200 || len(res.PurchaseId) != 26 {
		t.Fatalf("%+v %v", res, err)
	}
	if store.got.BuyerId != testSeller.Id {
		t.Fatalf("買い手を渡すこと: %+v", store.got)
	}
	if _, err := uc.Buy(testSeller, ""); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("ID が空: %v", err)
	}
	store.err = model.ErrAlreadySold
	if _, err := uc.Buy(testSeller, "ITEM1"); !errors.Is(err, model.ErrAlreadySold) {
		t.Fatalf("売り切れはそのまま返す: %v", err)
	}
}
