package usecase

import (
	"net/http"
	"time"

	"hackathon-backend/model"
)

// ImageMaxBytes は1枚の上限。画面で 768px の JPEG に縮めてから送るので、ふつうは 100KB 前後。
const ImageMaxBytes = 1_500_000

// ImagePerDayMax は1人が24時間に上げられる画像の枚数（DB で数えるので、サーバーの台数に関係なく効く）。
// 写真は選んだ時点で上がる（選び直した分も数える）ので、合計ではなく24時間ごとにする。
// 1枚は 768px の JPEG で 100KB 前後なので、100枚でも 10MB ほど。
const ImagePerDayMax = 100

// 受け付ける画像の形式（中身の先頭バイトで判定する。拡張子や申告は信用しない）。
var imageTypes = map[string]bool{"image/jpeg": true, "image/png": true, "image/webp": true}

// ImageStore は usecase が DB に求める操作。
type ImageStore interface {
	Insert(img model.Image) error
	// Get は無ければ model.ErrNotFound。
	Get(id string) (model.Image, error)
	// OwnerOf は画像を上げた人の ID。無ければ model.ErrNotFound。
	OwnerOf(id string) (string, error)
	// CountByOwnerSince はその人が since 以降に上げた画像の枚数。
	CountByOwnerSince(ownerId string, since time.Time) (int, error)
}

type ImageUsecase struct {
	Store ImageStore
	Limit *Limiter // 短い時間の回数制限（nil なら無し）
	Now   func() time.Time
}

// Upload は me の画像を保存して ID を返す。
func (uc *ImageUsecase) Upload(me model.Member, data []byte) (string, error) {
	if len(data) == 0 || len(data) > ImageMaxBytes {
		return "", ErrInvalidInput
	}
	ct := http.DetectContentType(data)
	if !imageTypes[ct] {
		return "", ErrInvalidInput
	}
	if !uc.Limit.Allow(me.Id) {
		return "", ErrTooMany
	}
	now := time.Now
	if uc.Now != nil {
		now = uc.Now
	}
	n, err := uc.Store.CountByOwnerSince(me.Id, now().Add(-24*time.Hour))
	if err != nil {
		return "", err
	}
	if n >= ImagePerDayMax {
		return "", model.ErrImageQuota
	}
	img := model.NewImage(me.Id, ct, data, now())
	if err := uc.Store.Insert(img); err != nil {
		return "", err
	}
	return img.Id, nil
}

func (uc *ImageUsecase) Get(id string) (model.Image, error) {
	if id == "" {
		return model.Image{}, model.ErrNotFound
	}
	return uc.Store.Get(id)
}
