package model

import (
	"sort"
	"testing"
	"time"
)

func TestConstructorsSetIdAndCreatedAt(t *testing.T) {
	at := time.Date(2026, 11, 11, 10, 20, 30, 999_000_000, time.UTC)
	want := time.Date(2026, 11, 11, 10, 20, 30, 0, time.UTC) // DATETIME は秒まで

	it := NewItem("教科書", "書き込みなし", 1200, Seller{Id: "S", DisplayName: "ota"}, "IMG", at)
	img := NewImage("U", "image/jpeg", []byte{1}, at)
	mem := NewMember("firebase-uid", "ota", at)
	pur := NewPurchase("ITEM", "BUYER", at)
	conv := NewConversation(Item{Id: "ITEM"}, "BUYER", at)
	msg := NewMessage("SENDER", "こんにちは", at)

	cases := []struct {
		name string
		id   string
		at   time.Time
	}{
		{"商品", it.Id, it.CreatedAt}, {"画像", img.Id, img.CreatedAt}, {"会員", mem.Id, mem.CreatedAt},
		{"購入", pur.Id, pur.CreatedAt}, {"やりとり", conv.Id, conv.CreatedAt}, {"メッセージ", msg.Id, msg.CreatedAt},
	}
	seen := map[string]bool{}
	for _, c := range cases {
		if len(c.id) != 26 || seen[c.id] {
			t.Errorf("%s: ID は 26 文字の ULID で、ほかと重ならない: %q", c.name, c.id)
		}
		seen[c.id] = true
		if !c.at.Equal(want) {
			t.Errorf("%s: 作成日時は秒に切りそろえる: %v", c.name, c.at)
		}
	}
	if it.Status != ItemOnSale || it.Seller == nil || it.Seller.Id != "S" || it.Price != 1200 || it.PhotoImageId != "IMG" {
		t.Errorf("商品は出品中で作り、渡した値を入れる: %+v", it)
	}
	if pur.Item.Id != "ITEM" || pur.BuyerId != "BUYER" || conv.BuyerId != "BUYER" || msg.SenderId != "SENDER" || mem.FirebaseUID != "firebase-uid" {
		t.Errorf("渡した値を入れる")
	}
}

// ULID を選んだ理由：作った順に並ぶ（メッセージの新着取得 ?after=<ID> で使う）
func TestIdsAreOrderedByCreation(t *testing.T) {
	ids := make([]string, 200)
	for i := range ids {
		ids[i] = NewMessage("S", "x", time.Now()).Id
	}
	if !sort.StringsAreSorted(ids) {
		t.Fatal("後から作った ID ほど大きい（文字列の順＝作った順）")
	}
}
