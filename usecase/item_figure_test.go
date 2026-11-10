package usecase

import (
	"errors"
	"strings"
	"testing"

	"hackathon-backend/model"
)

type fakeFigureStore struct {
	saved   bool // SaveFigure が返す値（MySQL は同じ値への UPDATE を 0 行と数える）
	buyer   string
	err     error
	gotJSON string
	gotOnly bool
}

func (f *fakeFigureStore) SaveFigure(id, js, caption string, onlyIfEmpty bool) (bool, error) {
	f.gotJSON, f.gotOnly = js, onlyIfEmpty
	return f.saved, f.err
}
func (f *fakeFigureStore) BuyerOf(string) (string, error) { return f.buyer, f.err }

func TestSaveFigureRules(t *testing.T) {
	seller := model.Member{Id: "S"}
	buyer := model.Member{Id: "B"}
	items := &fakeItemStore{items: []model.Item{
		{Id: "I1", Seller: &model.Seller{Id: "S"}},
		{Id: "SAMPLE"}, // 見本データ（出品者なし）
	}}
	fig := func() *Figure {
		return &Figure{Name: "本", Parts: []FigurePart{{Shape: "box", Size: []float64{1, 1, 1}, Pos: []float64{0, 0.5, 0}}}}
	}

	// 出品者：同じ模型の保存し直し（saved=false）も成功、上書きを許す
	st := &fakeFigureStore{saved: false}
	uc := &ItemUsecase{Store: items, Figures: st}
	if _, err := uc.SaveFigure(seller, "I1", fig()); err != nil || st.gotOnly {
		t.Fatalf("出品者: err=%v onlyIfEmpty=%v", err, st.gotOnly)
	}
	if strings.Contains(st.gotJSON, "engine") {
		t.Fatalf("engine は保存しない: %s", st.gotJSON)
	}
	// 購入者：空のときだけ。既にあれば ErrFigureExists
	st = &fakeFigureStore{saved: false, buyer: "B"}
	uc.Figures = st
	if _, err := uc.SaveFigure(buyer, "I1", fig()); !errors.Is(err, model.ErrFigureExists) || !st.gotOnly {
		t.Fatalf("購入者（既にある）: %v only=%v", err, st.gotOnly)
	}
	st.saved = true
	if _, err := uc.SaveFigure(buyer, "I1", fig()); err != nil {
		t.Fatalf("購入者（空）: %v", err)
	}
	// 見本データは出品者がいないので、買った人だけ
	st.buyer = ""
	if _, err := uc.SaveFigure(buyer, "SAMPLE", fig()); !errors.Is(err, model.ErrForbidden) {
		t.Fatalf("見本データを買っていない: %v", err)
	}
	// 入力の誤り
	if _, err := uc.SaveFigure(seller, "I1", nil); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("nil: %v", err)
	}
	if _, err := uc.SaveFigure(seller, "NOPE", fig()); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("無い商品: %v", err)
	}
	big := fig()
	for i := 0; i < 200; i++ {
		big.Parts = append(big.Parts, FigurePart{Shape: "sphere", Size: []float64{0.2}, Pos: []float64{0, 0, 0}})
	}
	if f, err := uc.SaveFigure(seller, "I1", big); err != nil || len(f.Parts) != FigurePartsMax {
		t.Fatalf("部品は %d 個までに切る: %v %d", FigurePartsMax, err, len(f.Parts))
	}
	// 札は6枚まで。細長い札（見た人のブラウザで巨大な絵になる）は幅と高さ÷幅を直す
	labels := fig()
	for i := 0; i < 20; i++ {
		labels.Parts = append(labels.Parts, FigurePart{Shape: "label", Size: []float64{0.0313, 4}, Pos: []float64{0, 0, 0}, Text: strings.Repeat("字", 50), Bg: "#000000"})
	}
	f, err := uc.SaveFigure(seller, "I1", labels)
	if err != nil || len(f.Parts) != 1+FigureLabelsMax {
		t.Fatalf("札は %d 枚まで: %v %d", FigureLabelsMax, err, len(f.Parts))
	}
	for _, p := range f.Parts[1:] {
		if p.Size[0] < 0.1 || p.Size[1] > p.Size[0] || p.Size[1] < p.Size[0]*0.1 || len([]rune(p.Text)) > FigureLabelMax {
			t.Fatalf("札の大きさ・文字数: %+v", p)
		}
	}

	// 売れた後は、出品者も「模型が無いときだけ」（購入者の箱庭の模型を差し替えない）
	sold := &fakeItemStore{items: []model.Item{{Id: "SOLD", Seller: &model.Seller{Id: "S"}, Status: model.ItemSold}}}
	st = &fakeFigureStore{saved: false}
	uc2 := &ItemUsecase{Store: sold, Figures: st}
	if _, err := uc2.SaveFigure(seller, "SOLD", fig()); !errors.Is(err, model.ErrFigureExists) || !st.gotOnly {
		t.Fatalf("売れた後の出品者の上書き: %v only=%v", err, st.gotOnly)
	}
	st.saved = true
	if _, err := uc2.SaveFigure(seller, "SOLD", fig()); err != nil {
		t.Fatalf("売れた後でも、模型が無ければ出品者は保存できる: %v", err)
	}
	// DB の失敗はそのまま返す
	uc.Figures = &fakeFigureStore{err: errors.New("db down")}
	if _, err := uc.SaveFigure(seller, "I1", fig()); err == nil || errors.Is(err, ErrInvalidInput) {
		t.Fatalf("DB の失敗: %v", err)
	}
}
