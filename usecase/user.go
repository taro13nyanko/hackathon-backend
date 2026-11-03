package usecase

import (
	"errors"
	"unicode/utf8"

	"github.com/oklog/ulid/v2"

	"hackathon-backend/model"
)

// ErrInvalidInput は入力が条件を満たさないとき（→ 400）。
var ErrInvalidInput = errors.New("invalid input")

// UserStore は usecase が DB に求める操作（テストで差し替えられるよう interface にしている）。
type UserStore interface {
	FindByName(name string) ([]model.User, error)
	FindAll() ([]model.User, error)
	Insert(u model.User) error
}

type UserUsecase struct {
	Store UserStore
}

func (uc *UserUsecase) Search(name string) ([]model.User, error) {
	if name == "" {
		return nil, ErrInvalidInput
	}
	return uc.Store.FindByName(name)
}

func (uc *UserUsecase) List() ([]model.User, error) {
	return uc.Store.FindAll()
}

func (uc *UserUsecase) Register(req model.UserCreateReq) (model.UserCreateRes, error) {
	// 文字数は len（バイト数）ではなく utf8.RuneCountInString で数える
	if req.Name == "" || utf8.RuneCountInString(req.Name) > 50 {
		return model.UserCreateRes{}, ErrInvalidInput
	}
	if req.Age < 20 || req.Age > 80 {
		return model.UserCreateRes{}, ErrInvalidInput
	}
	u := model.User{Id: ulid.Make().String(), Name: req.Name, Age: req.Age}
	if err := uc.Store.Insert(u); err != nil {
		return model.UserCreateRes{}, err
	}
	return model.UserCreateRes{Id: u.Id}, nil
}
