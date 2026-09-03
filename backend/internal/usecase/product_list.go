package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/minatomart/inventory-api/internal/domain"
)

type Pagination struct {
	Page    int
	PerPage int
}

// PaginationにTotalを入れてしまうと、リクエスト時にTotal:0を常に送ってしまう。
// そのため、レスポンス用に構造体を分離
type PageResult struct {
	Page    int
	PerPage int
	Total   int
}

// 詳細画面に遷移するためにIDを返す。
type ProductSummary struct {
	ID           string
	Name         string
	Status       domain.ProductStatus
	CategoryName string
	HasActiveSku bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// ProductListInput は一覧検索の絞り込み条件。 ゼロ値は「絞り込まない」を意味する。
type ProductListInput struct {
	Name       string
	SKUCode    string
	CategoryID string
	Pagination Pagination
}

type ProductListOutput struct {
	Items      []ProductSummary
	Pagination PageResult
}

type productUsecase struct {
	repo ProductRepository
}

func NewProductUsecase(repository ProductRepository) *productUsecase {
	return &productUsecase{
		repo: repository,
	}
}

func (p *productUsecase) ListProducts(ctx context.Context, input ProductListInput) (ProductListOutput, error) {
	products, total, err := p.repo.SearchProducts(ctx, input)

	if err != nil {
		return ProductListOutput{}, fmt.Errorf("fetch error: %w", err)
	}
	pageResult := PageResult{
		Page:    input.Pagination.Page,
		PerPage: input.Pagination.PerPage,
		Total:   total,
	}
	result := ProductListOutput{
		Items:      products,
		Pagination: pageResult,
	}
	return result, nil
}
