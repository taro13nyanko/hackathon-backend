package usecase

import (
	"strings"
	"time"
	"unicode/utf8"

	"hackathon-backend/model"
)

// DisplayNameMax は表示名の最大文字数。
const DisplayNameMax = 50

// MemberStore は usecase が DB に求める操作。
type MemberStore interface {
	// FindByFirebaseUID は無ければ model.ErrNotFound を返す。
	FindByFirebaseUID(uid string) (model.Member, error)
	// Insert は同じ firebase_uid が既にあれば model.ErrAlreadyRegistered を返す。
	Insert(m model.Member) error
}

type MemberUsecase struct {
	Store MemberStore
	Now   func() time.Time
}

// Me はログイン中の人のプロフィール。未登録なら model.ErrNotFound。
func (uc *MemberUsecase) Me(uid string) (model.Member, error) {
	return uc.Store.FindByFirebaseUID(uid)
}

// Register は初回ログイン時のプロフィール登録。
func (uc *MemberUsecase) Register(uid string, req model.MemberCreateReq) (model.Member, error) {
	name := strings.TrimSpace(req.DisplayName)
	if uid == "" || name == "" || utf8.RuneCountInString(name) > DisplayNameMax {
		return model.Member{}, ErrInvalidInput
	}
	now := time.Now
	if uc.Now != nil {
		now = uc.Now
	}
	m := model.NewMember(uid, name, now())
	if err := uc.Store.Insert(m); err != nil {
		return model.Member{}, err
	}
	return m, nil
}
