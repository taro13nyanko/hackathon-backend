package usecase

import (
	"net/http"
	"time"

	"github.com/oklog/ulid/v2"

	"hackathon-backend/model"
)

// ImageMaxBytes は1枚の上限。画面で 768px の JPEG に縮めてから送るので、ふつうは 100KB 前後。
const ImageMaxBytes = 1_500_000

// ImagePerUserMax は1人が上げられる画像の合計枚数（DB で数えるので、サーバーの台数に関係なく効く）。
// 1枚は 768px の JPEG で 100KB 前後なので、100枚でも 10MB ほど。
const ImagePerUserMax = 100

// 受け付ける画像の形式（中身の先頭バイトで判定する。拡張子や申告は信用しない）。
var imageTypes = map[string]bool{"image/jpeg": true, "image/png": true, "image/webp": true}

// ImageStore は usecase が DB に求める操作。
type ImageStore interface {
	Insert(img model.Image) error
	// Get は無ければ model.ErrNotFound。
	Get(id string) (model.Image, error)
	// OwnerOf は画像を上げた人の ID。無ければ model.ErrNotFound。
	OwnerOf(id string) (string, error)
	// CountByOwner はその人が上げた画像の枚数。
	CountByOwner(ownerId string) (int, error)
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
	n, err := uc.Store.CountByOwner(me.Id)
	if err != nil {
		return "", err
	}
	if n >= ImagePerUserMax {
		return "", ErrTooMany
	}
	now := time.Now
	if uc.Now != nil {
		now = uc.Now
	}
	img := model.Image{Id: ulid.Make().String(), OwnerId: me.Id, ContentType: ct, Data: data, CreatedAt: now().Truncate(time.Second)}
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
