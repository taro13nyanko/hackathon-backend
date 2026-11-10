package controller

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"hackathon-backend/model"
	"hackathon-backend/usecase"
)

// memGardenStore は dao.GardenDao をメモリで真似る（自分の箱庭の商品は own で決める）。
type memGardenStore struct {
	spots map[string][]model.GardenSpot
	own   map[string]bool
}

func (m *memGardenStore) Spots(userId string) ([]model.GardenSpot, error) {
	return m.spots[userId], nil
}
func (m *memGardenStore) GardenItemIds(string) (map[string]bool, error) { return m.own, nil }
func (m *memGardenStore) ReplaceSpots(userId string, s []model.GardenSpot, _ time.Time) error {
	m.spots[userId] = s
	return nil
}

func TestGardenAPI(t *testing.T) {
	members := &usecase.MemberUsecase{Store: &memMemberStore{}}
	a := &Auth{Verifier: fakeVerifier{}, Members: members}
	mux := http.NewServeMux()
	(&MeController{Usecase: members, Auth: a}).Register(mux)
	store := &memGardenStore{spots: map[string][]model.GardenSpot{}, own: map[string]bool{"I1": true}}
	(&GardenController{Usecase: &usecase.GardenUsecase{Store: store}, Auth: a}).Register(mux)
	h := CORS("", LimitBody(mux))
	doAuth(h, "POST", "/me", `{"display_name":"ota"}`, "tok-u")

	if rec := doAuth(h, "GET", "/me/garden", "", ""); rec.Code != 401 {
		t.Fatalf("ログインなし: %d", rec.Code)
	}
	if rec := doAuth(h, "GET", "/me/garden", "", "tok-u"); rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"spots":[]}` {
		t.Fatalf("最初は空: %d %s", rec.Code, rec.Body)
	}
	if rec := doAuth(h, "PUT", "/me/garden", `{"spots":[{"item_id":"I1","x":1.5,"z":-0.5}]}`, "tok-u"); rec.Code != 200 {
		t.Fatalf("保存: %d %s", rec.Code, rec.Body)
	}
	if rec := doAuth(h, "GET", "/me/garden", "", "tok-u"); !strings.Contains(rec.Body.String(), `"item_id":"I1","x":1.5,"z":-0.5`) {
		t.Fatalf("保存した配置: %s", rec.Body)
	}
	if rec := doAuth(h, "PUT", "/me/garden", `{"spots":[{"item_id":"NOT_MINE","x":0,"z":0}]}`, "tok-u"); rec.Code != 403 {
		t.Fatalf("自分の箱庭に無い商品は 403: %d", rec.Code)
	}
	if rec := doAuth(h, "PUT", "/me/garden", `{`, "tok-u"); rec.Code != 400 {
		t.Fatalf("JSON でない: %d", rec.Code)
	}
}
