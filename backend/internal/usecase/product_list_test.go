package usecase

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type mockProductRepository struct {
	Items []ProductSummary
	total int
	err   error
}

func (m *mockProductRepository) SearchProducts(ctx context.Context, input ProductListInput) ([]ProductSummary, int, error) {
	return m.Items, m.total, m.err
}

func Testレスポンスが商品情報とページネーションを返しているか(test *testing.T) {
	mock := &mockProductRepository{Items: []ProductSummary{
		{ID: "p-1", Name: "Tシャツ"},
		{ID: "p-2", Name: "ズボン"},
	},
		total: 2,
	}
	uc := NewProductUsecase(mock)
	input := ProductListInput{Pagination: Pagination{Page: 1, PerPage: 50}}
	got, err := uc.ListProducts(context.Background(), input)
	if err != nil {
		test.Fatalf("予期しないエラー: %v", err)
	}
	if !reflect.DeepEqual(got.Items, mock.Items) {
		test.Fatalf("items = %v, want %v", got.Items, mock.Items)
	}
	wantPage := PageResult{Page: 1, PerPage: 50, Total: 2}
	if got.Pagination != wantPage {
		test.Fatalf("pagination = %+v, want %+v", got.Pagination, wantPage)
	}

}

func Testリポジトリのエラーはラップして呼び出し元に返す(test *testing.T) {
	fetchErr := errors.New("フェッチエラー")
	mock := &mockProductRepository{Items: []ProductSummary{
		{ID: "p-1", Name: "Tシャツ"},
		{ID: "p-2", Name: "ズボン"},
	},
		total: 2,
		err:   fetchErr,
	}
	uc := NewProductUsecase(mock)
	input := ProductListInput{Pagination: Pagination{Page: 1, PerPage: 50}}
	got, err := uc.ListProducts(context.Background(), input)
	if !errors.Is(err, fetchErr) {
		test.Errorf("予期しないエラー: %v", err)
	}
	if !reflect.DeepEqual(got, ProductListOutput{}) {
		test.Fatalf("ProductListOutput = %+v, want %+v", got, ProductListOutput{})
	}
}
