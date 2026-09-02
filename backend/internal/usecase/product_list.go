package usecase

type Pagination struct {
	Page    int
	PerPage int
}
type ProductListInput struct {
	Name       string
	SKUCode    string
	CategoryID string
	Pagination Pagination
}

type ProductListOutput struct{}
