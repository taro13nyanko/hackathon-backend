package usecase

import (
	"math"
	"time"

	"hackathon-backend/model"
)

// 箱庭の配置：模型をつかんで動かした場所を保存する。
// 保存できるのは「自分の箱庭に並ぶ商品」＝自分が出品した商品と、自分が買った商品の位置だけ。

const (
	GardenSpotsMax = 200  // 1回に保存できる置き場所の数
	GardenCoordMax = 20.0 // 位置の大きさの上限（島の半径はふつう 3〜8 くらい）
)

// GardenStore は usecase が DB に求める操作（本物は dao.GardenDao）。
type GardenStore interface {
	Spots(userId string) ([]model.GardenSpot, error)
	// GardenItemIds は、その人の箱庭に並ぶ商品の ID（出品した商品と買った商品）。
	GardenItemIds(userId string) (map[string]bool, error)
	// ReplaceSpots は、その人の置き場所をまとめて入れ替える（1つのトランザクションで）。
	ReplaceSpots(userId string, spots []model.GardenSpot, at time.Time) error
}

type GardenUsecase struct {
	Store GardenStore
	Limit *Limiter // 保存の回数制限（nil なら無し）
	Now   func() time.Time
}

// Get は自分の箱庭の配置。まだ何も動かしていなければ空の配列。
func (uc *GardenUsecase) Get(me model.Member) (model.GardenLayout, error) {
	spots, err := uc.Store.Spots(me.Id)
	if err != nil {
		return model.GardenLayout{}, err
	}
	if spots == nil {
		spots = []model.GardenSpot{}
	}
	return model.GardenLayout{Spots: spots}, nil
}

// Save は自分の箱庭の配置をまとめて保存する（前の配置は消える）。空なら「きれいに並べ直す」。
func (uc *GardenUsecase) Save(me model.Member, req model.GardenLayout) (model.GardenLayout, error) {
	if len(req.Spots) > GardenSpotsMax {
		return model.GardenLayout{}, ErrInvalidInput
	}
	if !uc.Limit.Allow(me.Id) {
		return model.GardenLayout{}, ErrTooMany
	}
	own, err := uc.Store.GardenItemIds(me.Id)
	if err != nil {
		return model.GardenLayout{}, err
	}
	out := make([]model.GardenSpot, 0, len(req.Spots))
	seen := map[string]bool{}
	for _, s := range req.Spots {
		if s.ItemId == "" || seen[s.ItemId] || !finite(s.X) || !finite(s.Z) {
			return model.GardenLayout{}, ErrInvalidInput
		}
		if !own[s.ItemId] {
			return model.GardenLayout{}, model.ErrForbidden // 自分の箱庭に無い商品
		}
		seen[s.ItemId] = true
		out = append(out, model.GardenSpot{
			ItemId: s.ItemId,
			X:      round3(clamp(s.X, -GardenCoordMax, GardenCoordMax)),
			Z:      round3(clamp(s.Z, -GardenCoordMax, GardenCoordMax)),
		})
	}
	now := time.Now
	if uc.Now != nil {
		now = uc.Now
	}
	if err := uc.Store.ReplaceSpots(me.Id, out, now().Truncate(time.Second)); err != nil {
		return model.GardenLayout{}, err
	}
	return model.GardenLayout{Spots: out}, nil
}

func round3(x float64) float64 { return math.Round(x*1000) / 1000 }
