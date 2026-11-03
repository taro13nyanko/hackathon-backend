package model

import "errors"

// ErrNotFound は該当するデータがないとき（→ 404）。
// dao が返し、controller が 404 に変える。どちらからも使うので model に置く。
var ErrNotFound = errors.New("not found")
