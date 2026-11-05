package usecase

import (
	"errors"
	"strings"
	"testing"

	"hackathon-backend/model"
)

// fakeDMStore は1つのやりとり（買い手 b・出品者 s）だけを持つ。
type fakeDMStore struct {
	msgs []model.Message
}

func (f *fakeDMStore) FindOrCreate(c model.Conversation) (string, bool, error) {
	return "C1", true, nil
}
func (f *fakeDMStore) Get(id, viewer string) (model.Conversation, error) {
	return model.Conversation{Id: id}, nil
}
func (f *fakeDMStore) ListFor(string) ([]model.Conversation, error) { return nil, nil }
func (f *fakeDMStore) Participants(id string) (string, string, error) {
	if id != "C1" {
		return "", "", model.ErrNotFound
	}
	return "b", "s", nil
}
func (f *fakeDMStore) Messages(string, string) ([]model.Message, error) { return f.msgs, nil }
func (f *fakeDMStore) InsertMessage(_ string, m model.Message) error {
	f.msgs = append(f.msgs, m)
	return nil
}

func TestDM(t *testing.T) {
	items := &fakeItemStore{items: []model.Item{
		{Id: "I1", Seller: &model.Seller{Id: "s"}},
		{Id: "SAMPLE"}, // 見本データ（出品者なし）
	}}
	uc := &DMUsecase{Store: &fakeDMStore{}, Items: items}
	buyer, seller, other := model.Member{Id: "b"}, model.Member{Id: "s"}, model.Member{Id: "x"}

	if _, created, err := uc.Start(buyer, "I1"); err != nil || !created {
		t.Fatalf("始める: %v %v", created, err)
	}
	if _, _, err := uc.Start(seller, "I1"); !errors.Is(err, model.ErrOwnItem) {
		t.Fatalf("自分の商品: %v", err)
	}
	if _, _, err := uc.Start(buyer, "SAMPLE"); !errors.Is(err, model.ErrNoSeller) {
		t.Fatalf("出品者なし: %v", err)
	}
	if _, _, err := uc.Start(buyer, "NOPE"); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("無い商品: %v", err)
	}

	if m, err := uc.Send(buyer, "C1", model.MessageCreateReq{Body: "  まだありますか？ "}); err != nil || m.Body != "まだありますか？" || m.SenderId != "b" {
		t.Fatalf("送る: %+v %v", m, err)
	}
	if _, err := uc.Send(seller, "C1", model.MessageCreateReq{Body: "あります"}); err != nil {
		t.Fatalf("出品者も送れる: %v", err)
	}
	for _, body := range []string{"", "   ", strings.Repeat("あ", MessageBodyMax+1)} {
		if _, err := uc.Send(buyer, "C1", model.MessageCreateReq{Body: body}); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("本文 %d 文字: %v", len(body), err)
		}
	}
	if _, err := uc.Send(other, "C1", model.MessageCreateReq{Body: "hi"}); !errors.Is(err, model.ErrForbidden) {
		t.Fatalf("参加者以外は送れない: %v", err)
	}
	if _, err := uc.Messages(other, "C1", ""); !errors.Is(err, model.ErrForbidden) {
		t.Fatalf("参加者以外は読めない: %v", err)
	}
	if _, err := uc.Messages(buyer, "C9", ""); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("無いやりとり: %v", err)
	}
	if ms, err := uc.Messages(seller, "C1", ""); err != nil || len(ms) != 2 {
		t.Fatalf("読む: %v %v", ms, err)
	}
}
