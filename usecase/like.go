package usecase

import (
	"time"

	"hackathon-backend/model"
)

// いいね：PUT /items/{id}/like で付け、DELETE で外す（どちらも何回呼んでも同じ結果＝冪等）。
// 自分の商品にも付けられる。非公開（hidden）の商品は 404。

// LikePerMinute は1人1分あたりのいいね・取り消しの回数。
const LikePerMinute = 60

// LikeStore は usecase が DB に求める操作（本物は dao.LikeDao）。
type LikeStore interface {
	// Like はいいねを付ける（もう付いていれば何もしない）。
	Like(userId, itemId string, at time.Time) error
	// Unlike はいいねを外す（付いていなくても成功）。
	Unlike(userId, itemId string) error
	// CountLikes は商品のいいねの数。
	CountLikes(itemId string) (int, error)
	// FindLikedBy はいいねした商品（いいねした新しい順、hidden は除く）。
	FindLikedBy(userId string) ([]model.Item, error)
}

type LikeUsecase struct {
	Store LikeStore
	Items ItemStore
	Limit *Limiter // 回数制限（nil なら無し）
	Now   func() time.Time
}

// Like は me が商品 itemId にいいねする。
func (uc *LikeUsecase) Like(me model.Member, itemId string) (model.LikeRes, error) {
	return uc.set(me, itemId, true)
}

// Unlike は me のいいねを外す。
func (uc *LikeUsecase) Unlike(me model.Member, itemId string) (model.LikeRes, error) {
	return uc.set(me, itemId, false)
}

func (uc *LikeUsecase) set(me model.Member, itemId string, liked bool) (model.LikeRes, error) {
	if me.Id == "" {
		return model.LikeRes{}, ErrInvalidInput
	}
	// 回数は DB を読む前に数える（無い商品・非公開の商品の連打で DB を読ませ続けない）
	if !uc.Limit.Allow(me.Id) {
		return model.LikeRes{}, ErrTooMany
	}
	if _, err := findActive(uc.Items, itemId, me.Id); err != nil {
		return model.LikeRes{}, err
	}
	var err error
	if liked {
		now := time.Now
		if uc.Now != nil {
			now = uc.Now
		}
		err = uc.Store.Like(me.Id, itemId, now().Truncate(time.Second))
	} else {
		err = uc.Store.Unlike(me.Id, itemId)
	}
	if err != nil {
		return model.LikeRes{}, err
	}
	n, err := uc.Store.CountLikes(itemId)
	if err != nil {
		return model.LikeRes{}, err
	}
	return model.LikeRes{Liked: liked, LikeCount: n}, nil
}

// List は me がいいねした商品（いいねした新しい順、100件、hidden は除く）。
func (uc *LikeUsecase) List(me model.Member) ([]model.Item, error) {
	items, err := uc.Store.FindLikedBy(me.Id)
	if err != nil {
		return nil, err
	}
	return redact(items, &me), nil
}
