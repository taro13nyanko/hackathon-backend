package usecase

import (
	"encoding/json"

	"hackathon-backend/model"
)

// 模型の設計図を商品に保存する（箱庭に並べるため）。
//
// 保存できるのは
//   - 出品者：いつでも（作り直したら上書き）
//   - 購入者：まだ模型が無いときだけ（出品者の模型を上書きしない。既にあれば 409 figure_exists）
//
// それ以外の人は 403。設計図は POST /figures と同じ決まり（ParseFigure）で範囲内に直してから保存する。

// FigureJSONMax は保存する設計図の大きさの上限（部品 60 個でも 20KB 前後なので十分）。
const FigureJSONMax = 64 << 10

// ItemFigureStore は模型の保存に使う DB の操作（本物は dao.ItemDao）。
type ItemFigureStore interface {
	SaveFigure(id, figureJSON, caption string, onlyIfEmpty bool) (bool, error)
	BuyerOf(itemId string) (string, error)
}

// SaveFigure は商品 itemId に模型 f を保存し、保存した（直した後の）設計図を返す。
func (uc *ItemUsecase) SaveFigure(me model.Member, itemId string, f *Figure) (Figure, error) {
	if itemId == "" || f == nil || uc.Figures == nil {
		return Figure{}, ErrInvalidInput
	}
	it, err := uc.Store.FindById(itemId)
	if err != nil {
		return Figure{}, err
	}

	// 誰が保存できるか
	onlyIfEmpty := false
	if it.Seller == nil || it.Seller.Id != me.Id {
		buyer, err := uc.Figures.BuyerOf(itemId)
		if err != nil {
			return Figure{}, err
		}
		if buyer == "" || buyer != me.Id {
			return Figure{}, model.ErrForbidden
		}
		onlyIfEmpty = true
	}

	// 画面から来た設計図も、Gemini の出力と同じ決まりで直す（形・数値の範囲・部品数・顔の番号）
	raw, err := json.Marshal(f)
	if err != nil {
		return Figure{}, ErrInvalidInput
	}
	clean, err := ParseFigure(string(raw))
	if err != nil {
		return Figure{}, ErrInvalidInput
	}
	clean.Engine = ""
	js, err := json.Marshal(clean)
	if err != nil || len(js) > FigureJSONMax {
		return Figure{}, ErrInvalidInput
	}

	saved, err := uc.Figures.SaveFigure(itemId, string(js), clean.Caption, onlyIfEmpty)
	if err != nil {
		return Figure{}, err
	}
	if !saved && onlyIfEmpty {
		return Figure{}, model.ErrFigureExists
	}
	// 出品者で saved=false は「同じ模型を保存し直した」（MySQL は変わらない行を数えない）。商品はあるので成功でよい
	return clean, nil
}
