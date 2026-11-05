package usecase

import (
	"errors"
	"strings"
	"testing"

	"hackathon-backend/model"
)

type fakeMemberStore struct{ members []model.Member }

func (f *fakeMemberStore) FindByFirebaseUID(uid string) (model.Member, error) {
	for _, m := range f.members {
		if m.FirebaseUID == uid {
			return m, nil
		}
	}
	return model.Member{}, model.ErrNotFound
}

func (f *fakeMemberStore) Insert(m model.Member) error {
	if _, err := f.FindByFirebaseUID(m.FirebaseUID); err == nil {
		return model.ErrAlreadyRegistered
	}
	f.members = append(f.members, m)
	return nil
}

func TestRegisterMember(t *testing.T) {
	uc := &MemberUsecase{Store: &fakeMemberStore{}}

	if _, err := uc.Me("uid-1"); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("未登録は ErrNotFound: %v", err)
	}
	m, err := uc.Register("uid-1", model.MemberCreateReq{DisplayName: "  ota  "})
	if err != nil || m.DisplayName != "ota" || len(m.Id) != 26 {
		t.Fatalf("登録: %+v %v", m, err)
	}
	if got, err := uc.Me("uid-1"); err != nil || got.Id != m.Id {
		t.Fatalf("登録後の Me: %+v %v", got, err)
	}
	if _, err := uc.Register("uid-1", model.MemberCreateReq{DisplayName: "again"}); !errors.Is(err, model.ErrAlreadyRegistered) {
		t.Fatalf("2回目は ErrAlreadyRegistered: %v", err)
	}
}

func TestRegisterMemberValidation(t *testing.T) {
	uc := &MemberUsecase{Store: &fakeMemberStore{}}
	for _, name := range []string{"", "   ", strings.Repeat("あ", DisplayNameMax+1)} {
		if _, err := uc.Register("uid-x", model.MemberCreateReq{DisplayName: name}); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%q: want ErrInvalidInput, got %v", name, err)
		}
	}
	if _, err := uc.Register("uid-x", model.MemberCreateReq{DisplayName: strings.Repeat("あ", DisplayNameMax)}); err != nil {
		t.Errorf("%d文字ちょうどは通る: %v", DisplayNameMax, err)
	}
}
