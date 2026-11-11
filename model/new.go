package model

import (
	"time"

	"github.com/oklog/ulid/v2"
)

// ---- 新しい記録を作るコンストラクタ ----
//
// 方針（UTTC コード設計ワークショップの模範解答＝GRASP の「生成者」に合わせる）：
//   - ID と作成日時は、model のコンストラクタ（NewXxx）が決める。
//   - 呼び出すのは usecase だけ。controller・dao・DB・画面では ID を作らない。
//   - どの記録も同じ決め方にそろえる（機能ごとに作る場所がばらばらだと、読みにくくバグの元になる）。
//
// ID は UUID ではなく ULID（26文字）。作った順に並ぶので、メッセージの新着取得（?after=<ID>）と
// 同じ時刻どうしの並び順に使っている（UUID v4 は順番がばらばらで使えない）。
//
// 作成日時は「呼んだ時刻」。テストで時刻を固定できるよう、usecase の時計の値を at で受け取る。
// DB の DATETIME は秒までなので、ここで秒に切りそろえる。

// NewID は新しい ID（ULID）を返す。
func NewID() string { return ulid.Make().String() }

func stamp(at time.Time) time.Time { return at.Truncate(time.Second) }

// NewItem は出品する商品（出品中の状態で作る）。
func NewItem(title, description string, price int, seller Seller, photoImageId string, at time.Time) Item {
	return Item{
		Id:           NewID(),
		Title:        title,
		Description:  description,
		Price:        price,
		Status:       ItemOnSale,
		Seller:       &seller,
		PhotoImageId: photoImageId,
		CreatedAt:    stamp(at),
	}
}

// NewImage は上げられた画像。
func NewImage(ownerId, contentType string, data []byte, at time.Time) Image {
	return Image{Id: NewID(), OwnerId: ownerId, ContentType: contentType, Data: data, CreatedAt: stamp(at)}
}

// NewMember はプロフィール登録した利用者。
func NewMember(firebaseUID, displayName string, at time.Time) Member {
	return Member{Id: NewID(), FirebaseUID: firebaseUID, DisplayName: displayName, CreatedAt: stamp(at)}
}

// NewPurchase は買った記録（値段は買う瞬間の商品の値段を、DB のトランザクションの中で入れる）。
func NewPurchase(itemId, buyerId string, at time.Time) Purchase {
	return Purchase{Id: NewID(), Item: Item{Id: itemId}, BuyerId: buyerId, CreatedAt: stamp(at)}
}

// NewConversation は商品と買い手ごとのやりとり。
func NewConversation(item Item, buyerId string, at time.Time) Conversation {
	return Conversation{Id: NewID(), Item: item, BuyerId: buyerId, CreatedAt: stamp(at)}
}

// NewMessage はやりとりの中のメッセージ。
func NewMessage(senderId, body string, at time.Time) Message {
	return Message{Id: NewID(), SenderId: senderId, Body: body, CreatedAt: stamp(at)}
}
