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

var (
	// ErrForbidden はやりとりの参加者でない人が見ようとしたとき（→ 403）。
	ErrForbidden = errors.New("forbidden")
	// ErrNoSeller は出品者がいない商品（見本データ）にメッセージを送ろうとしたとき（→ 409）。
	ErrNoSeller = errors.New("no seller")
)

// ErrImageQuota は24時間に上げられる画像の枚数を超えたとき（→ 429 image_quota）。
var ErrImageQuota = errors.New("image quota")

// ErrFigureExists は購入者が模型を保存しようとしたが、既に模型があるとき（→ 409）。出品者の模型は上書きしない。
var ErrFigureExists = errors.New("figure exists")
