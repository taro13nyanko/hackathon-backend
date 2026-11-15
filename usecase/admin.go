package usecase

import (
	"strings"
	"unicode/utf8"

	"hackathon-backend/model"
)

// 管理者の操作：要確認（review）・非公開（hidden）の商品を見て、公開（ok）か非公開（hidden）かを決める。
// controller の RequireAdmin でも確かめるが、usecase でも管理者かを確かめる（入口を増やしても漏れないように）。

// ModerationNoteMax は管理者のメモの最大文字数。
const ModerationNoteMax = 120

// ModerationStore は管理者の操作に使う DB の操作（本物は dao.ItemDao）。
type ModerationStore interface {
	// FindByModeration は確認の状態が status の商品（新しい順に100件）。
	FindByModeration(status, viewerId string) ([]model.Item, error)
	// SetModeration は確認の状態を変える。reason が nil なら理由はそのまま。無ければ model.ErrNotFound。
	SetModeration(id, status string, reason *string) error
}

type AdminUsecase struct {
	Store ModerationStore
	Items ItemStore
}

// Queue は管理画面の一覧。status は review（既定）か hidden。
func (uc *AdminUsecase) Queue(me model.Member, status string) ([]model.Item, error) {
	if !me.IsAdmin() {
		return nil, model.ErrForbidden
	}
	switch status {
	case "":
		status = model.ModerationReview
	case model.ModerationReview, model.ModerationHidden:
	default:
		return nil, ErrInvalidInput
	}
	return uc.Store.FindByModeration(status, me.Id)
}

// SetModeration は商品 itemId を公開（ok）か非公開（hidden）にして、更新後の商品を返す。
// note があれば理由をそれで上書きする（無ければ理由はそのまま）。
func (uc *AdminUsecase) SetModeration(me model.Member, itemId string, req model.ModerationUpdateReq) (model.Item, error) {
	if !me.IsAdmin() {
		return model.Item{}, model.ErrForbidden
	}
	note := strings.TrimSpace(req.Note)
	if itemId == "" || utf8.RuneCountInString(note) > ModerationNoteMax {
		return model.Item{}, ErrInvalidInput
	}
	if req.Status != model.ModerationOK && req.Status != model.ModerationHidden {
		return model.Item{}, ErrInvalidInput
	}
	if _, err := uc.Items.FindById(itemId, me.Id); err != nil {
		return model.Item{}, err
	}
	var reason *string
	if note != "" {
		reason = &note
	}
	if err := uc.Store.SetModeration(itemId, req.Status, reason); err != nil {
		return model.Item{}, err
	}
	return uc.Items.FindById(itemId, me.Id)
}
