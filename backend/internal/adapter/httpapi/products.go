package httpapi

import (
	"math"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/minatomart/inventory-api/internal/domain"
	"github.com/minatomart/inventory-api/internal/usecase"
)

// openapi.yaml の Page/PerPage 定義に合わせる)
const (
	defaultPage    = 1
	defaultPerPage = 50
	minPage        = 1
	maxPage        = math.MaxInt // 上限は仕様にないため制限しない
	minPerPage     = 1
	maxPerPage     = 50
)

// handleListProducts は商品一覧を返す。
// 商品名部分一致。SKUコードは前方一致で絞り込み可能
// ページネーションは1page50件とする
func (s *Server) handleListProducts(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	skuCode := r.URL.Query().Get("skuCode")
	categoryID := r.URL.Query().Get("categoryId")
	if categoryID != "" {
		_, err := uuid.Parse(categoryID)
		if err != nil {
			writeError(w, r, &domain.ValidationError{Field: "categoryId", Message: "UUID形式で指定してください"})
			return
		}
	}

	p, err := parseIntParam(r.URL.Query().Get("page"), "page", defaultPage, minPage, maxPage)
	if err != nil {
		writeError(w, r, err)
		return
	}

	per, err := parseIntParam(r.URL.Query().Get("perPage"), "perPage", defaultPerPage, minPerPage, maxPerPage)
	if err != nil {
		writeError(w, r, err)
		return
	}

	input := usecase.ProductListInput{
		Name:       name,
		SKUCode:    skuCode,
		CategoryID: categoryID,
		Pagination: usecase.Pagination{
			Page:    p,
			PerPage: per,
		},
	}
	output, err := s.products.ListProducts(r.Context(), input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, output)
}

// parseIntParam は数値クエリパラメータを検証して返す。省略時は def を返す。
// HTTPを知らない純関数にして、エラー応答の書き込みは呼び出し元に残す。
func parseIntParam(raw, field string, def, min, max int) (int, error) {
	if raw == "" {
		return def, nil
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed < min || parsed > max {
		return 0, &domain.ValidationError{Field: field, Message: "不正な" + field + "が入力されました"}
	}
	return parsed, nil
}
