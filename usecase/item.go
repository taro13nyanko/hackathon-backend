package usecase

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/oklog/ulid/v2"

	"hackathon-backend/model"
)

// 出品の決まり（壊れてはいけないので item_test.go で固定している）。
const (
	ItemTitleMax       = 40
	ItemDescriptionMax = 1000
	ItemPriceMin       = 300
	ItemPriceMax       = 9_999_999
)

// ItemStore は usecase が DB に求める操作（テストで差し替えられるよう interface にしている）。
type ItemStore interface {
	FindAll(keyword string) ([]model.Item, error)
	FindById(id string) (model.Item, error)
	FindBySeller(sellerId string) ([]model.Item, error)
	Insert(it model.Item) error
}

type ItemUsecase struct {
	Store  ItemStore
	Images ImageStore       // 写真が本人のものか確かめるため
	Now    func() time.Time // テストで時刻を固定するため。nil なら time.Now
}

func (uc *ItemUsecase) now() time.Time {
	if uc.Now != nil {
		return uc.Now()
	}
	return time.Now()
}

// List は商品一覧。keyword は前後の空白を除いて使う（空なら全件）。
func (uc *ItemUsecase) List(keyword string) ([]model.Item, error) {
	return uc.Store.FindAll(strings.TrimSpace(keyword))
}

// Mine は自分が出品した商品（新しい順）。
func (uc *ItemUsecase) Mine(me model.Member) ([]model.Item, error) {
	return uc.Store.FindBySeller(me.Id)
}

// Get は商品1件。見つからなければ model.ErrNotFound がそのまま返る。
func (uc *ItemUsecase) Get(id string) (model.Item, error) {
	if id == "" {
		return model.Item{}, ErrInvalidInput
	}
	return uc.Store.FindById(id)
}

// Create は出品する。seller はログイン中の人（プロフィール登録済み）。
func (uc *ItemUsecase) Create(seller model.Member, req model.ItemCreateReq) (model.ItemCreateRes, error) {
	if seller.Id == "" {
		return model.ItemCreateRes{}, ErrInvalidInput
	}
	title := strings.TrimSpace(req.Title)
	desc := strings.TrimSpace(req.Description)
	if title == "" || utf8.RuneCountInString(title) > ItemTitleMax {
		return model.ItemCreateRes{}, ErrInvalidInput
	}
	if utf8.RuneCountInString(desc) > ItemDescriptionMax {
		return model.ItemCreateRes{}, ErrInvalidInput
	}
	if req.Price < ItemPriceMin || req.Price > ItemPriceMax {
		return model.ItemCreateRes{}, ErrInvalidInput
	}
	// 写真は任意。付けるなら、自分が上げた写真に限る
	if req.PhotoImageId != "" {
		if uc.Images == nil {
			return model.ItemCreateRes{}, ErrInvalidInput
		}
		owner, err := uc.Images.OwnerOf(req.PhotoImageId)
		if err != nil || owner != seller.Id {
			return model.ItemCreateRes{}, ErrInvalidInput
		}
	}
	it := model.Item{
		Id:           ulid.Make().String(),
		Title:        title,
		Description:  desc,
		Price:        req.Price,
		Status:       model.ItemOnSale,
		Seller:       &model.Seller{Id: seller.Id, DisplayName: seller.DisplayName},
		PhotoImageId: req.PhotoImageId,
		CreatedAt:    uc.now().Truncate(time.Second), // DATETIME は秒まで
	}
	if err := uc.Store.Insert(it); err != nil {
		return model.ItemCreateRes{}, err
	}
	return model.ItemCreateRes{Id: it.Id}, nil
}
