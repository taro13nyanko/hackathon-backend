package usecase

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"hackathon-backend/model"
)

type fakeImageStore struct{ imgs map[string]model.Image }

func (f *fakeImageStore) Insert(img model.Image) error {
	if f.imgs == nil {
		f.imgs = map[string]model.Image{}
	}
	f.imgs[img.Id] = img
	return nil
}
func (f *fakeImageStore) Get(id string) (model.Image, error) {
	if img, ok := f.imgs[id]; ok {
		return img, nil
	}
	return model.Image{}, model.ErrNotFound
}
func (f *fakeImageStore) CountByOwner(owner string) (int, error) {
	n := 0
	for _, img := range f.imgs {
		if img.OwnerId == owner {
			n++
		}
	}
	return n, nil
}

func (f *fakeImageStore) OwnerOf(id string) (string, error) {
	img, err := f.Get(id)
	return img.OwnerId, err
}

// 最小の PNG（1x1）。先頭の8バイトで PNG と判定される
var tinyPNG = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89\x00\x00\x00\rIDATx\x9cc\x00\x01\x00\x00\x05\x00\x01\r\n-\xb4\x00\x00\x00\x00IEND\xaeB`\x82")

func TestUploadImage(t *testing.T) {
	store := &fakeImageStore{}
	uc := &ImageUsecase{Store: store}
	me := model.Member{Id: "m1"}

	id, err := uc.Upload(me, tinyPNG)
	if err != nil || len(id) != 26 {
		t.Fatalf("PNG: %q %v", id, err)
	}
	if img, _ := uc.Get(id); img.ContentType != "image/png" || img.OwnerId != "m1" {
		t.Fatalf("形式と持ち主を記録: %+v", img)
	}
	for name, data := range map[string][]byte{
		"空":     nil,
		"テキスト":  []byte("hello, this is not an image"),
		"HTML":  []byte("<html><script>alert(1)</script></html>"),
		"大きすぎる": append(append([]byte{}, tinyPNG...), bytes.Repeat([]byte{0}, ImageMaxBytes)...),
	} {
		if _, err := uc.Upload(me, data); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%s: want ErrInvalidInput, got %v", name, err)
		}
	}
	if _, err := uc.Get("NOPE"); !errors.Is(err, model.ErrNotFound) {
		t.Errorf("無い画像: %v", err)
	}
}

func TestCreateItemWithPhoto(t *testing.T) {
	imgs := &fakeImageStore{}
	mine, _ := (&ImageUsecase{Store: imgs}).Upload(testSeller, tinyPNG)
	others, _ := (&ImageUsecase{Store: imgs}).Upload(model.Member{Id: "someone"}, tinyPNG)
	store := &fakeItemStore{}
	uc := &ItemUsecase{Store: store, Images: imgs}

	if _, err := uc.Create(testSeller, model.ItemCreateReq{Title: "机", Price: 1000, PhotoImageId: mine}); err != nil {
		t.Fatal(err)
	}
	if store.items[0].PhotoImageId != mine {
		t.Errorf("写真を付けること: %+v", store.items[0])
	}
	for name, id := range map[string]string{"他人の写真": others, "無い写真": "NOPE"} {
		if _, err := uc.Create(testSeller, model.ItemCreateReq{Title: "机", Price: 1000, PhotoImageId: id}); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%s: want ErrInvalidInput, got %v", name, err)
		}
	}
}

func TestUploadImageLimits(t *testing.T) {
	jpeg := append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, make([]byte, 64)...)
	store := &fakeImageStore{}
	uc := &ImageUsecase{Store: store}
	me := model.Member{Id: "M1"}
	for i := 0; i < ImagePerUserMax; i++ {
		if _, err := uc.Upload(me, jpeg); err != nil {
			t.Fatalf("%d 枚目: %v", i+1, err)
		}
	}
	if _, err := uc.Upload(me, jpeg); !errors.Is(err, ErrTooMany) {
		t.Fatalf("%d 枚を超えたら ErrTooMany: %v", ImagePerUserMax, err)
	}
	if _, err := uc.Upload(model.Member{Id: "M2"}, jpeg); err != nil {
		t.Fatalf("ほかの人は上げられる: %v", err)
	}
	uc2 := &ImageUsecase{Store: &fakeImageStore{}, Limit: &Limiter{Max: 2, Window: time.Minute}}
	uc2.Upload(me, jpeg)
	uc2.Upload(me, jpeg)
	if _, err := uc2.Upload(me, jpeg); !errors.Is(err, ErrTooMany) {
		t.Fatalf("回数制限: %v", err)
	}
}
