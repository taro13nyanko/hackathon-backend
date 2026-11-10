package usecase

import (
	"errors"
	"math"
	"testing"
	"time"

	"hackathon-backend/model"
)

type fakeGardenStore struct {
	spots map[string][]model.GardenSpot
	own   map[string]map[string]bool
	err   error
}

func (f *fakeGardenStore) Spots(userId string) ([]model.GardenSpot, error) {
	return f.spots[userId], f.err
}
func (f *fakeGardenStore) GardenItemIds(userId string) (map[string]bool, error) {
	return f.own[userId], f.err
}
func (f *fakeGardenStore) ReplaceSpots(userId string, spots []model.GardenSpot, _ time.Time) error {
	if f.err != nil {
		return f.err
	}
	if f.spots == nil {
		f.spots = map[string][]model.GardenSpot{}
	}
	f.spots[userId] = spots
	return nil
}

func TestGarden(t *testing.T) {
	me := model.Member{Id: "U1"}
	store := &fakeGardenStore{own: map[string]map[string]bool{"U1": {"SOLD": true, "BOUGHT": true}}}
	uc := &GardenUsecase{Store: store}

	got, err := uc.Get(me)
	if err != nil || got.Spots == nil || len(got.Spots) != 0 {
		t.Fatalf("まだ動かしていなければ空の配列（null ではなく）: %+v %v", got, err)
	}

	saved, err := uc.Save(me, model.GardenLayout{Spots: []model.GardenSpot{
		{ItemId: "SOLD", X: 1.23456, Z: -2},
		{ItemId: "BOUGHT", X: 999, Z: -999},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if s := saved.Spots; len(s) != 2 || s[0].X != 1.235 || s[1].X != GardenCoordMax || s[1].Z != -GardenCoordMax {
		t.Fatalf("位置は丸めて、範囲内に直す: %+v", saved.Spots)
	}
	if got, _ := uc.Get(me); len(got.Spots) != 2 {
		t.Fatalf("保存した配置が読める: %+v", got)
	}

	bad := []struct {
		name  string
		spots []model.GardenSpot
		want  error
	}{
		{"自分の箱庭に無い商品", []model.GardenSpot{{ItemId: "OTHER", X: 0, Z: 0}}, model.ErrForbidden},
		{"商品 ID が空", []model.GardenSpot{{ItemId: "", X: 0, Z: 0}}, ErrInvalidInput},
		{"同じ商品が2回", []model.GardenSpot{{ItemId: "SOLD"}, {ItemId: "SOLD"}}, ErrInvalidInput},
		{"数でない位置", []model.GardenSpot{{ItemId: "SOLD", X: math.NaN()}}, ErrInvalidInput},
		{"無限大の位置", []model.GardenSpot{{ItemId: "SOLD", Z: math.Inf(1)}}, ErrInvalidInput},
	}
	for _, c := range bad {
		if _, err := uc.Save(me, model.GardenLayout{Spots: c.spots}); !errors.Is(err, c.want) {
			t.Errorf("%s: want %v, got %v", c.name, c.want, err)
		}
	}
	many := make([]model.GardenSpot, GardenSpotsMax+1)
	if _, err := uc.Save(me, model.GardenLayout{Spots: many}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("%d 個を超える: %v", GardenSpotsMax, err)
	}

	// 空で保存すると、並べ直し（配置が消える）
	if _, err := uc.Save(me, model.GardenLayout{}); err != nil {
		t.Fatal(err)
	}
	if got, _ := uc.Get(me); len(got.Spots) != 0 {
		t.Fatalf("空で保存したら配置は消える: %+v", got)
	}

	// 回数制限と DB の失敗
	limited := &GardenUsecase{Store: store, Limit: &Limiter{Max: 1, Window: time.Minute}}
	limited.Save(me, model.GardenLayout{})
	if _, err := limited.Save(me, model.GardenLayout{}); !errors.Is(err, ErrTooMany) {
		t.Fatalf("回数制限: %v", err)
	}
	broken := &GardenUsecase{Store: &fakeGardenStore{err: errors.New("db down")}}
	if _, err := broken.Save(me, model.GardenLayout{}); err == nil {
		t.Fatal("DB の失敗はそのまま返す")
	}
}
