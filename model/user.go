package model

// User は user テーブルの1行。
type User struct {
	Id   string `json:"id"`
	Name string `json:"name"`
	Age  int    `json:"age"`
}

// UserCreateReq は POST /user のリクエストボディ。
type UserCreateReq struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

// UserCreateRes は POST /user のレスポンス。
type UserCreateRes struct {
	Id string `json:"id"`
}
