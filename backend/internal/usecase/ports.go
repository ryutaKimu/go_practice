package usecase

import "context"

type ProductUsecase interface {
	ListProducts(ctx context.Context, input ProductListInput) (ProductListOutput, error)
}
