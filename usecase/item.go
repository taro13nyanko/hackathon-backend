package usecase

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

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
// 商品には、いいねの数（LikeCount）と、viewer がいいね済みか（Liked）を入れて返す。
type ItemStore interface {
	// FindAll は一覧（hidden は除く。新しい順）。
	FindAll(q model.ItemQuery) ([]model.Item, error)
	// FindById は1件（hidden も返す）。無ければ model.ErrNotFound。viewerId は liked の計算に使う（空なら false）。
	FindById(id, viewerId string) (model.Item, error)
	// FindBySeller は出品者の商品（hidden も含む。新しい順）。
	FindBySeller(sellerId string) ([]model.Item, error)
	Insert(it model.Item) error
}

type ItemUsecase struct {
	Store     ItemStore
	Images    ImageStore       // 写真が本人のものか確かめるため（確認では写真を Gemini に見せる）
	Figures   ItemFigureStore  // 模型の保存（PUT /items/{id}/figure）。nil なら保存できない
	Moderator Moderator        // 出品の確認（禁止物の検知）。nil なら確認しない（ok）
	Limit     *Limiter         // 出品の回数制限（nil なら無し）
	Now       func() time.Time // テストで時刻を固定するため。nil なら time.Now
}

func (uc *ItemUsecase) now() time.Time {
	if uc.Now != nil {
		return uc.Now()
	}
	return time.Now()
}

// viewerId は見ている人の ID（未ログインは空文字）。
func viewerId(viewer *model.Member) string {
	if viewer == nil {
		return ""
	}
	return viewer.Id
}

// canSeeModeration は viewer が商品の確認の理由を見てよいか（出品者本人と管理者だけ）。
// hidden の商品を見てよい人も同じ。
func canSeeModeration(it model.Item, viewer *model.Member) bool {
	if viewer == nil || viewer.Id == "" {
		return false
	}
	return viewer.IsAdmin() || (it.Seller != nil && it.Seller.Id == viewer.Id)
}

// redact は、理由を見てはいけない人には理由を空にする（状態は見せてよい）。
func redact(items []model.Item, viewer *model.Member) []model.Item {
	for i := range items {
		if !canSeeModeration(items[i], viewer) {
			items[i].ModerationReason = ""
		}
	}
	return items
}

// List は商品一覧（hidden は除く）。viewer は未ログインなら nil（q.ViewerId はここで入れ直す）。
//   - q.Keyword は前後の空白を除いて使う（空なら全件）。
//   - q.MinPrice・q.MaxPrice は値段の範囲（0 は指定なし）。負なら ErrInvalidInput、上限より上は上限に収め、
//     両方あって min>max なら入れ替える（音声検索の結果をそのまま渡せるように）。
func (uc *ItemUsecase) List(viewer *model.Member, q model.ItemQuery) ([]model.Item, error) {
	if q.MinPrice < 0 || q.MaxPrice < 0 {
		return nil, ErrInvalidInput
	}
	q.MinPrice, q.MaxPrice = min(q.MinPrice, ItemPriceMax), min(q.MaxPrice, ItemPriceMax)
	if q.MinPrice > 0 && q.MaxPrice > 0 && q.MinPrice > q.MaxPrice {
		q.MinPrice, q.MaxPrice = q.MaxPrice, q.MinPrice
	}
	q.Keyword, q.ViewerId = strings.TrimSpace(q.Keyword), viewerId(viewer)
	items, err := uc.Store.FindAll(q)
	if err != nil {
		return nil, err
	}
	return redact(items, viewer), nil
}

// Mine は自分が出品した商品（新しい順。hidden も理由つきで見える）。
func (uc *ItemUsecase) Mine(me model.Member) ([]model.Item, error) {
	items, err := uc.Store.FindBySeller(me.Id)
	if err != nil {
		return nil, err
	}
	return redact(items, &me), nil
}

// Get は商品1件。見つからなければ model.ErrNotFound がそのまま返る。
// hidden の商品は出品者本人と管理者にだけ返し、ほかの人には model.ErrNotFound。
func (uc *ItemUsecase) Get(viewer *model.Member, id string) (model.Item, error) {
	if id == "" {
		return model.Item{}, ErrInvalidInput
	}
	it, err := uc.Store.FindById(id, viewerId(viewer))
	if err != nil {
		return model.Item{}, err
	}
	if it.IsHidden() && !canSeeModeration(it, viewer) {
		return model.Item{}, model.ErrNotFound
	}
	return redact([]model.Item{it}, viewer)[0], nil
}

// findActive は購入・やりとり・いいね・模型の保存の対象になる商品。hidden は model.ErrNotFound（だれでも）。
func findActive(store ItemStore, id, viewerId string) (model.Item, error) {
	if id == "" {
		return model.Item{}, ErrInvalidInput
	}
	it, err := store.FindById(id, viewerId)
	if err != nil {
		return model.Item{}, err
	}
	if it.IsHidden() {
		return model.Item{}, model.ErrNotFound
	}
	return it, nil
}

// Create は出品する。seller はログイン中の人（プロフィール登録済み）。
// 保存の前に Moderator で確認し、明らかな違反は *ProhibitedError（保存しない）、迷う物は review で保存する。
func (uc *ItemUsecase) Create(ctx context.Context, seller model.Member, req model.ItemCreateReq) (model.ItemCreateRes, error) {
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
	if !uc.Limit.Allow(seller.Id) {
		return model.ItemCreateRes{}, ErrTooMany
	}
	// 出品の確認（Gemini）。回数制限の後に呼ぶ（呼びすぎを防ぐ）
	status, reason, err := uc.moderate(ctx, title, desc, req.PhotoImageId)
	if err != nil {
		return model.ItemCreateRes{}, err
	}
	// ID・作成日時・出品中の状態は model のコンストラクタが決める
	it := model.NewItem(title, desc, req.Price, model.Seller{Id: seller.Id, DisplayName: seller.DisplayName}, req.PhotoImageId, uc.now())
	it.ModerationStatus, it.ModerationReason = status, reason
	if err := uc.Store.Insert(it); err != nil {
		return model.ItemCreateRes{}, err
	}
	return model.ItemCreateRes{Id: it.Id, ModerationStatus: it.ModerationStatus, ModerationReason: it.ModerationReason}, nil
}
