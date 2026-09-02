package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/minatomart/inventory-api/internal/usecase"
)

type mockProductUsecase struct {
	gotInput usecase.ProductListInput
}

func (m *mockProductUsecase) ListProducts(ctx context.Context, input usecase.ProductListInput) (usecase.ProductListOutput, error) {
	m.gotInput = input
	return usecase.ProductListOutput{}, nil
}

func Test商品一覧検索_省略時はデフォルト値で検索される(t *testing.T) {
	mock := &mockProductUsecase{}
	srv := NewServer(stubDB{}, "test", nil, mock)
	t.Run("省略時はデフォルト値で検索され,200を返す", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/products", nil)
		srv.Routes().ServeHTTP(rec, req)

		if got := mock.gotInput.Pagination.Page; got != defaultPage {
			t.Errorf("page = %d, want %d", got, defaultPage)
		}

		if got := mock.gotInput.Pagination.PerPage; got != defaultPerPage {
			t.Errorf("perPage = %d, want %d", got, defaultPerPage)
		}

		if got := rec.Result().StatusCode; got != http.StatusOK {
			t.Errorf("status = %d, want 200 ", got)
		}
	})
}

func Test商品一覧検索_指定した検索条件がusecaseに渡される(t *testing.T) {
	mock := &mockProductUsecase{}
	srv := NewServer(stubDB{}, "test", nil, mock)

	categoryID := "0e2a3b52-9df1-4a49-9e56-6bfa43a12345"
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/products?name=バスマット&skuCode=MM-TOWEL&categoryId="+categoryID, nil)
	srv.Routes().ServeHTTP(rec, req)

	if got := rec.Result().StatusCode; got != http.StatusOK {
		t.Fatalf("status = %d, want 200", got)
	}
	if got := mock.gotInput.Name; got != "バスマット" {
		t.Errorf("name = %q, want %q", got, "バスマット")
	}
	if got := mock.gotInput.SKUCode; got != "MM-TOWEL" {
		t.Errorf("skuCode = %q, want %q", got, "MM-TOWEL")
	}
	if got := mock.gotInput.CategoryID; got != categoryID {
		t.Errorf("categoryId = %q, want %q", got, categoryID)
	}
}

func Test商品一覧検索_不正な検索条件は400を返す(t *testing.T) {
	tests := []struct {
		name       string
		queryParam string
		wantStatus int
	}{
		{"pageが文字列", "page=test", 400},
		{"pageが0", "page=0", 400},
		{"pageが負の数", "page=-1", 400},
		{"perPageが文字列", "perPage=test", 400},
		{"perPageが最大値を超える", "perPage=51", 400},
		{"perPageが0", "perPage=0", 400},
		{"perPageが負の数", "perPage=-1", 400},
		{"uuidが文字列", "categoryId=aa", 400},
	}
	mock := &mockProductUsecase{}
	srv := NewServer(stubDB{}, "test", nil, mock)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/products?%s", tt.queryParam), nil)
			srv.Routes().ServeHTTP(rec, req)
			if got := rec.Result().StatusCode; got != tt.wantStatus {
				t.Errorf("status = %d, want %d ", got, tt.wantStatus)
			}
		})
	}
}
