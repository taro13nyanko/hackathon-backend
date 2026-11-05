package model

import "errors"

// ErrNotFound は該当するデータがないとき（→ 404）。
// dao が返し、controller が 404 に変える。どちらからも使うので model に置く。
var ErrNotFound = errors.New("not found")

// 購入できないとき（→ 409）。
var (
	ErrAlreadySold = errors.New("already sold") // 売り切れ
	ErrOwnItem     = errors.New("own item")     // 自分が出品した商品
)
