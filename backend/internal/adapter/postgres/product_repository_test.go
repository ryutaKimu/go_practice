package postgres

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	uc "github.com/minatomart/inventory-api/internal/usecase"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

var testDB *DB

func TestMain(m *testing.M) {
	files, err := filepath.Glob("../../../migrations/*.up.sql")
	if err != nil {
		log.Fatalf("ファイル取得に失敗: %v", err)
	}

	dbName := postgres.WithDatabase("minato")
	userName := postgres.WithUsername("minato")
	password := postgres.WithPassword("minato_local_only")

	// postgresのコンテナ起動はinitScript適用後に停止し、その後本起動する2段階形式。
	// BasicWaitStrategiesがないと、仮起動段階でDBアクセスを行い、弾かれることがある
	pgContainer, err := postgres.Run(context.Background(), "postgres:16", postgres.WithInitScripts(files...), dbName, userName, password, postgres.BasicWaitStrategies())

	if err != nil {
		log.Fatalf("コンテナ起動に失敗: %v", err)
	}

	dsn, err := pgContainer.ConnectionString(context.Background(), "sslmode=disable")
	if err != nil {
		log.Fatalf("DSN取得に失敗: %v", err)
	}
	db, err := Connect(context.Background(), dsn, 3*time.Second)
	if err != nil {
		log.Fatalf("db接続に失敗: %v", err)
	}
	testDB = db
	code := m.Run()
	pgContainer.Container.Terminate(context.Background())
	os.Exit(code)
}

func Test商品一覧が全件とカテゴリ名を返す(test *testing.T) {
	repo := NewProductRepository(testDB)
	input := uc.ProductListInput{
		Pagination: uc.Pagination{
			Page:    1,
			PerPage: 50,
		},
	}
	items, total, err := repo.SearchProducts(context.Background(), input)
	if err != nil {
		test.Fatalf("予期しないエラー: %v", err)
	}

	var want int
	q := `SELECT COUNT(*) FROM products`
	err = testDB.Pool().QueryRow(context.Background(), q).Scan(&want)
	if err != nil {
		test.Fatalf("データの件数取得失敗:%v", err)
	}

	if want == 0 {
		test.Fatalf("シーダデータが存在していない")
	}

	if want > 50 {
		test.Fatalf("シーダデータが50件を超えています")
	}

	if total != want {
		test.Fatalf("total=%+v, want %+v", total, want)
	}
	if len(items) != want {
		test.Fatalf("ProductSummary=%+v, want %+v", len(items), want)
	}

	for _, i := range items {
		if i.CategoryName == "" {
			test.Fatalf("カテゴリー名が空:商品ID=%s", i.ID)
		}

		if i.Status == "" {
			test.Fatalf("販売ステータスが空:商品ID=%s", i.ID)
		}
	}
}

func Test商品検索が絞り込めているか(t *testing.T) {
	repo := NewProductRepository(testDB)
	var categoryId string
	err := testDB.Pool().QueryRow(context.Background(), `INSERT INTO categories (name) VALUES ($1) RETURNING id`, "テスト用カテゴリ").Scan(&categoryId)
	if err != nil {
		t.Fatalf("読み込み失敗:%v", err)
	}

	var productId string
	err = testDB.Pool().QueryRow(context.Background(), `INSERT INTO products (name, category_id) VALUES ($1, $2) RETURNING id`,
		"ふわふわバスタオル", categoryId).Scan(&productId)
	if err != nil {
		t.Fatalf("商品INSERT失敗(%s):%v", "ふわふわバスタオル", err)
	}
	_, err = testDB.Pool().Exec(context.Background(), `INSERT INTO products (name, category_id) VALUES ($1, $2)`,
		"マグカップ", categoryId)
	if err != nil {
		t.Fatalf("商品INSERT失敗(%s):%v", "マグカップ", err)
	}

	_, err = testDB.Pool().Exec(context.Background(),
		`INSERT INTO skus (product_id, code, price_amount) VALUES ($1, $2, $3)`,
		productId, "TEST-TOWEL", 1500)

	if err != nil {
		t.Fatalf("SKU INSERT失敗(%s):%v", "TEST-TOWEL", err)
	}

	t.Cleanup(func() {
		testDB.Pool().Exec(context.Background(),
			`DELETE FROM products WHERE category_id = $1`, categoryId)
		testDB.Pool().Exec(context.Background(),
			`DELETE FROM categories WHERE id = $1`, categoryId)
	})

	tests := []struct {
		testCase string
		input    uc.ProductListInput
		wantHit  []string
		wantMiss []string
	}{
		{testCase: "商品名検索", input: uc.ProductListInput{Name: "タオル", Pagination: uc.Pagination{Page: 1, PerPage: 50}}, wantHit: []string{"ふわふわバスタオル"}, wantMiss: []string{"マグカップ"}},
		{testCase: "SKUコード検索", input: uc.ProductListInput{Name: "", SKUCode: "TEST-TOWEL", Pagination: uc.Pagination{Page: 1, PerPage: 50}}, wantHit: []string{"ふわふわバスタオル"}, wantMiss: []string{"マグカップ"}},
	}

	for _, tt := range tests {
		t.Run(tt.testCase, func(t *testing.T) {
			items, _, err := repo.SearchProducts(context.Background(), tt.input)
			if err != nil {
				t.Fatalf("予期しないエラー: %v", err)
			}

			for _, name := range tt.wantHit {
				if !containsProduct(items, name) {
					t.Errorf("ヒットすべき商品がいない: %s", name)
				}
			}
			for _, name := range tt.wantMiss {
				if containsProduct(items, name) {
					t.Errorf("絞り込みから漏れている: %s", name)
				}
			}
		})
	}
}

func containsProduct(items []uc.ProductSummary, name string) bool {
	for _, item := range items {
		if item.Name == name {
			return true
		}
	}
	return false
}
