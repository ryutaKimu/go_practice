package postgres

import (
	"context"
	"fmt"
	"strings"

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
	whereSQL, args := buildProductQueryWhere(input)
	// O-4案B: 全SKU停止中の商品も一覧に残し、警告表示用のフラグとして返す（WHEREに移すと商品が消える）
	baseSelect := `
	SELECT
		products.id,
		products.name,
		products.status,
		products.created_at,
		products.updated_at,
		categories.name,
		EXISTS (SELECT 1 FROM skus WHERE skus.product_id = products.id AND skus.status = 'active') AS has_active_sku
	FROM products
	INNER JOIN categories ON products.category_id = categories.id
	`
	countQ := `SELECT COUNT(*) FROM products ` + whereSQL
	var total int
	err := r.DB.Pool().QueryRow(ctx, countQ, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count products data: %w", err)
	}

	page := input.Pagination.Page
	perPage := input.Pagination.PerPage
	args = append(args, perPage, (page-1)*perPage)
	//同時刻の場合、product.idで優先順位をつける。
	q := baseSelect + whereSQL + fmt.Sprintf(" ORDER BY products.created_at DESC, products.id LIMIT $%d OFFSET $%d", len(args)-1, len(args))
	rows, err := r.DB.Pool().Query(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("fetch products data: %w", err)
	}
	defer rows.Close()
	var items []uc.ProductSummary
	for rows.Next() {
		var p uc.ProductSummary
		if err := rows.Scan(&p.ID, &p.Name, &p.Status, &p.CreatedAt, &p.UpdatedAt, &p.CategoryName, &p.HasActiveSku); err != nil {
			return nil, 0, fmt.Errorf("fetch products data: %w", err)
		}
		items = append(items, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("fetch products data: %w", err)
	}
	return items, total, nil
}

func buildProductQueryWhere(input uc.ProductListInput) (string, []any) {
	where := []string{}
	args := []any{}

	if input.Name != "" {
		args = append(args, "%"+input.Name+"%")
		where = append(where, fmt.Sprintf("products.name ILIKE $%d", len(args)))
	}

	if input.SKUCode != "" {
		args = append(args, input.SKUCode+"%")
		where = append(where, fmt.Sprintf("EXISTS(SELECT 1 FROM skus WHERE skus.product_id = products.id AND skus.code LIKE $%d)", len(args)))
	}

	if input.CategoryID != "" {
		args = append(args, input.CategoryID)
		where = append(where, fmt.Sprintf(`products.category_id IN (
			WITH RECURSIVE tree AS (
			  SELECT id FROM categories WHERE id = $%d
			  UNION ALL
			  SELECT c.id FROM categories c JOIN tree ON c.parent_id = tree.id
			)
			SELECT id FROM tree
			)`, len(args)))
	}

	if len(where) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(where, " AND "), args
}
