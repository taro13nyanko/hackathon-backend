package usecase

import (
	"errors"
	"strings"
	"testing"

	"hackathon-backend/model"
)

type fakeStore struct {
	inserted []model.User
	err      error
}

func (f *fakeStore) FindByName(name string) ([]model.User, error) {
	out := []model.User{}
	for _, u := range f.inserted {
		if u.Name == name {
			out = append(out, u)
		}
	}
	return out, f.err
}

func (f *fakeStore) FindAll() ([]model.User, error) {
	return append([]model.User{}, f.inserted...), f.err
}

func (f *fakeStore) Insert(u model.User) error {
	if f.err != nil {
		return f.err
	}
	f.inserted = append(f.inserted, u)
	return nil
}

func TestRegisterValidation(t *testing.T) {
	cases := []struct {
		name string
		req  model.UserCreateReq
		ok   bool
	}{
		{"正常", model.UserCreateReq{Name: "hanako", Age: 20}, true},
		{"上限80", model.UserCreateReq{Name: "taro", Age: 80}, true},
		{"19歳", model.UserCreateReq{Name: "taro", Age: 19}, false},
		{"81歳", model.UserCreateReq{Name: "taro", Age: 81}, false},
		{"名前空", model.UserCreateReq{Name: "", Age: 30}, false},
		{"日本語50文字(150バイト)はOK", model.UserCreateReq{Name: strings.Repeat("あ", 50), Age: 30}, true},
		{"51文字はNG", model.UserCreateReq{Name: strings.Repeat("あ", 51), Age: 30}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			uc := &UserUsecase{Store: &fakeStore{}}
			res, err := uc.Register(c.req)
			if c.ok {
				if err != nil || len(res.Id) != 26 {
					t.Fatalf("want ok with 26-char id, got id=%q err=%v", res.Id, err)
				}
			} else if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("want ErrInvalidInput, got %v", err)
			}
		})
	}
}

func TestRegisterThenSearch(t *testing.T) {
	uc := &UserUsecase{Store: &fakeStore{}}
	res, err := uc.Register(model.UserCreateReq{Name: "hanako", Age: 25})
	if err != nil {
		t.Fatal(err)
	}
	users, err := uc.Search("hanako")
	if err != nil || len(users) != 1 || users[0].Id != res.Id {
		t.Fatalf("got %v %v", users, err)
	}
	if _, err := uc.Search(""); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("empty name should be invalid, got %v", err)
	}
}

func TestStoreErrorPropagates(t *testing.T) {
	boom := errors.New("db down")
	uc := &UserUsecase{Store: &fakeStore{err: boom}}
	if _, err := uc.Register(model.UserCreateReq{Name: "a", Age: 30}); !errors.Is(err, boom) {
		t.Fatalf("want db error, got %v", err)
	}
}

func TestList(t *testing.T) {
	uc := &UserUsecase{Store: &fakeStore{}}
	if users, err := uc.List(); err != nil || users == nil || len(users) != 0 {
		t.Fatalf("empty store should give empty non-nil slice, got %v %v", users, err)
	}
	uc.Register(model.UserCreateReq{Name: "a", Age: 30})
	uc.Register(model.UserCreateReq{Name: "b", Age: 40})
	if users, _ := uc.List(); len(users) != 2 {
		t.Fatalf("want 2 users, got %d", len(users))
	}
}
