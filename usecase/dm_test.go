package usecase

import (
	"errors"
	"strings"
	"testing"
	"time"

	"hackathon-backend/model"
)

// fakeDMStore は1つのやりとり C1（買い手 b・出品者 s）だけを持つ。
// 既読の位置は DAO と同じ決まり（実在するメッセージの ID だけ・後戻りしない）で真似る。
type fakeDMStore struct {
	msgs      []model.Message
	reads     map[string]string // 人 → 既読の位置
	unreadErr error             // 未読を数えるときに返すエラー（sql/009 を流す前の再現）
}

func (f *fakeDMStore) FindOrCreate(c model.Conversation) (string, bool, error) {
	return "C1", true, nil
}
func (f *fakeDMStore) Get(id, viewer string) (model.Conversation, error) {
	return model.Conversation{Id: id}, nil
}
func (f *fakeDMStore) ListFor(viewer string) ([]model.Conversation, error) {
	if viewer != "b" && viewer != "s" {
		return []model.Conversation{}, nil
	}
	return []model.Conversation{{Id: "C1"}}, nil
}
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
func (f *fakeDMStore) UnreadCounts(viewer string) (map[string]int, error) {
	if f.unreadErr != nil {
		return nil, f.unreadErr
	}
	out := map[string]int{}
	if viewer != "b" && viewer != "s" {
		return out, nil
	}
	for _, m := range f.msgs {
		if m.SenderId != viewer && m.Id > f.reads[viewer] {
			out["C1"]++
		}
	}
	return out, nil
}
func (f *fakeDMStore) MarkRead(_, user, upTo string, _ time.Time) (string, error) {
	if f.reads == nil {
		f.reads = map[string]string{}
	}
	target := ""
	for _, m := range f.msgs {
		if (upTo == "" || m.Id <= upTo) && m.Id > target {
			target = m.Id
		}
	}
	if target > f.reads[user] {
		f.reads[user] = target
	}
	return f.reads[user], nil
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

// 未読の数え方と既読の付け方（問題3：新着に未読バッジが付かない）。
func TestDMUnread(t *testing.T) {
	store := &fakeDMStore{msgs: []model.Message{
		{Id: "01A", SenderId: "b", Body: "まだありますか？"},
		{Id: "01B", SenderId: "s", Body: "あります"},
		{Id: "01C", SenderId: "s", Body: "値下げもできます"},
	}}
	uc := &DMUsecase{Store: store}
	buyer, seller, other := model.Member{Id: "b"}, model.Member{Id: "s"}, model.Member{Id: "x"}

	// 相手から来た分だけが未読（自分が送った分は数えない）
	cs, err := uc.List(buyer)
	if err != nil || len(cs) != 1 || cs[0].UnreadCount != 2 {
		t.Fatalf("買い手の一覧: %+v %v", cs, err)
	}
	if cs, _ := uc.List(seller); cs[0].UnreadCount != 1 {
		t.Fatalf("出品者の一覧: %+v", cs)
	}
	if u, err := uc.Unread(buyer); err != nil || u.Count != 2 {
		t.Fatalf("買い手の合計: %+v %v", u, err)
	}

	// 画面に出した所まで既読にする（それより新しい分は未読のまま）
	res, err := uc.MarkRead(buyer, "C1", model.ReadReq{LastMessageId: "01B"})
	if err != nil || res.LastReadMessageId != "01B" || res.UnreadCount != 1 {
		t.Fatalf("01B まで既読: %+v %v", res, err)
	}
	// 省略すれば最新まで
	if res, err := uc.MarkRead(buyer, "C1", model.ReadReq{}); err != nil || res.LastReadMessageId != "01C" || res.UnreadCount != 0 {
		t.Fatalf("最新まで既読: %+v %v", res, err)
	}
	// 古い ID を渡しても後戻りしない
	if res, _ := uc.MarkRead(buyer, "C1", model.ReadReq{LastMessageId: "01A"}); res.LastReadMessageId != "01C" || res.UnreadCount != 0 {
		t.Fatalf("後戻りした: %+v", res)
	}
	// 既読は人ごと（買い手が読んでも、出品者の未読は減らない）
	if u, _ := uc.Unread(seller); u.Count != 1 {
		t.Fatalf("出品者の合計: %+v", u)
	}
	// 新しいメッセージが来たらまた未読
	store.msgs = append(store.msgs, model.Message{Id: "01D", SenderId: "s", Body: "送料込みです"})
	if u, _ := uc.Unread(buyer); u.Count != 1 {
		t.Fatalf("新着: %+v", u)
	}

	// 参加者以外は 403、無いやりとりは 404、長すぎる ID は 400
	if _, err := uc.MarkRead(other, "C1", model.ReadReq{}); !errors.Is(err, model.ErrForbidden) {
		t.Fatalf("参加者以外: %v", err)
	}
	if _, err := uc.MarkRead(buyer, "C9", model.ReadReq{}); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("無いやりとり: %v", err)
	}
	if _, err := uc.MarkRead(buyer, "C1", model.ReadReq{LastMessageId: strings.Repeat("Z", 27)}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("長すぎる ID: %v", err)
	}
	if u, _ := uc.Unread(other); u.Count != 0 {
		t.Fatalf("参加していない人の未読: %+v", u)
	}
}

// sql/009 を流す前に backend を出しても、一覧は返る（壊れるのは未読だけ）。
func TestDMListWithoutReadsTable(t *testing.T) {
	store := &fakeDMStore{
		msgs:      []model.Message{{Id: "01A", SenderId: "s", Body: "こんにちは"}},
		unreadErr: errors.New("Error 1146: Table 'hackathon.conversation_reads' doesn't exist"),
	}
	uc := &DMUsecase{Store: store}
	cs, err := uc.List(model.Member{Id: "b"})
	if err != nil || len(cs) != 1 || cs[0].UnreadCount != 0 {
		t.Fatalf("一覧は返す: %+v %v", cs, err)
	}
	if _, err := uc.Unread(model.Member{Id: "b"}); err == nil {
		t.Fatal("合計は失敗を返す（500）")
	}
	if _, err := uc.MarkRead(model.Member{Id: "b"}, "C1", model.ReadReq{}); err == nil {
		t.Fatal("既読も失敗を返す（500）")
	}
}
