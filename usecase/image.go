package usecase

import (
	"net/http"
	"time"

	"github.com/oklog/ulid/v2"

	"hackathon-backend/model"
)

// ImageMaxBytes は1枚の上限。画面で 768px の JPEG に縮めてから送るので、ふつうは 100KB 前後。
const ImageMaxBytes = 1_500_000

// 受け付ける画像の形式（中身の先頭バイトで判定する。拡張子や申告は信用しない）。
var imageTypes = map[string]bool{"image/jpeg": true, "image/png": true, "image/webp": true}

// ImageStore は usecase が DB に求める操作。
type ImageStore interface {
	Insert(img model.Image) error
	// Get は無ければ model.ErrNotFound。
	Get(id string) (model.Image, error)
	// OwnerOf は画像を上げた人の ID。無ければ model.ErrNotFound。
	OwnerOf(id string) (string, error)
}

type ImageUsecase struct {
	Store ImageStore
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
