package model

// GardenSpot は箱庭での模型1つの置き場所（島の中心からの位置。y は上なので x と z だけ）。
type GardenSpot struct {
	ItemId string  `json:"item_id"`
	X      float64 `json:"x"`
	Z      float64 `json:"z"`
}

// GardenLayout は GET/PUT /me/garden の本文。spots に無い模型は、画面が決めた場所に並ぶ。
type GardenLayout struct {
	Spots []GardenSpot `json:"spots"`
}
