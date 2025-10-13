package types

type CreatePublicKeyRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	PublicKey   string `json:"publicKey"`
}

type (
	ListPublicKeysRequest struct {
		Offset int    `form:"offset" binding:"omitempty,min=0"`
		Limit  int    `form:"limit" binding:"omitempty,min=1"`
		SortBy string `form:"sortBy" binding:"omitempty"`
		Order  string `form:"order" binding:"omitempty,oneof=desc asc"`
		UserId string `form:"-"`
	}
	ListPublicKeysResponse struct {
		TotalCount int                          `json:"totalCount"`
		Items      []ListPublicKeysResponseItem `json:"items"`
	}
	ListPublicKeysResponseItem struct {
		Id          int64  `json:"id"`
		UserId      string `json:"userId"`
		Description string `json:"description"`
		PublicKey   string `json:"publicKey"`
		Status      bool   `json:"status"`
		CreateTime  string `json:"createTime"`
		UpdateTime  string `json:"updateTime"`
	}
)

type SetPublicKeyStatusRequest struct {
	Status bool `json:"status"`
}
type SetPublicKeyDescriptionRequest struct {
	Description string `json:"description"`
}
