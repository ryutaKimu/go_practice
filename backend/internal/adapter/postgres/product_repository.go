package postgres

import (
	"context"
	"fmt"

	uc "github.com/minatomart/inventory-api/internal/usecase"
)

type productRepository struct {
	DB *DB
}

var _ uc.ProductRepository = (*productRepository)(nil)

func NewProductRepository(db *DB) *productRepository {
	return &productRepository{
		DB: db,
	}
}

func (r *productRepository) SearchProducts(ctx context.Context, input uc.ProductListInput) ([]uc.ProductSummary, int, error) {
	q := `
	SELECT
		products.id,
		products.name,
		products.status,
		products.created_at,
		products.updated_at,
		categories.name
	FROM products
	INNER JOIN categories ON products.category_id = categories.id
	ORDER BY products.created_at DESC, products.id
	LIMIT $1 OFFSET $2
	`
	page := input.Pagination.Page
	perPage := input.Pagination.PerPage
	rows, err := r.DB.Pool().Query(ctx, q, perPage, (page-1)*perPage)
	if err != nil {
		return nil, 0, fmt.Errorf("fetch products data: %w", err)
	}
	defer rows.Close()
	var items []uc.ProductSummary
	for rows.Next() {
		var p uc.ProductSummary
		if err := rows.Scan(&p.ID, &p.Name, &p.Status, &p.CreatedAt, &p.UpdatedAt, &p.CategoryName); err != nil {
			return nil, 0, fmt.Errorf("fetch products data: %w", err)
		}
		items = append(items, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("fetch products data: %w", err)
	}
	var total int
	err = r.DB.Pool().QueryRow(ctx, "SELECT count(*) from products").Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count products data: %w", err)
	}

	return items, total, nil
}
