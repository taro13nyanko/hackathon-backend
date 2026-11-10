package usecase

import "errors"

// ErrInvalidInput は入力が条件を満たさないとき（→ 400）。
var ErrInvalidInput = errors.New("invalid input")

// ErrTooMany は短い時間に呼びすぎたとき・上限に達したとき（→ 429）。
var ErrTooMany = errors.New("too many requests")
