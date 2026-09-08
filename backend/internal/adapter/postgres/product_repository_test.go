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

func Test全SKUが停止中の商品はhasActiveSkuがfalseになる(t *testing.T) {
	repo := NewProductRepository(testDB)

	ctx := context.Background()

	var categoryId string
	err := testDB.Pool().QueryRow(ctx, `INSERT INTO categories (name) VALUES ($1) RETURNING id`, "テスト用カテゴリ").Scan(&categoryId)
	if err != nil {
		t.Fatalf("カテゴリINSERT失敗:%v", err)
	}

	t.Cleanup(func() {
		testDB.Pool().Exec(ctx, `DELETE FROM products WHERE category_id = $1`, categoryId)
		testDB.Pool().Exec(ctx, `DELETE FROM categories WHERE id = $1`, categoryId)
	})

	// hasActiveSku が false になる経路は「全SKUが停止中」と「SKUが1件も無い」の2通りある。
	// SQLの書き方によっては後者だけ取りこぼすため、両方を用意する。
	fixtures := []struct {
		productName string
		skuCode     string // 空文字ならSKUを作らない
		skuStatus   string
	}{
		{"販売中タオル", "TEST-ACTIVE-001", "active"},
		{"全停止タオル", "TEST-SUSPENDED-001", "suspended"},
		{"SKUなしタオル", "", ""},
	}

	for _, f := range fixtures {
		var productId string
		err := testDB.Pool().QueryRow(ctx,
			`INSERT INTO products (name, category_id) VALUES ($1, $2) RETURNING id`,
			f.productName, categoryId).Scan(&productId)
		if err != nil {
			t.Fatalf("商品INSERT失敗(%s):%v", f.productName, err)
		}

		if f.skuCode == "" {
			continue
		}

		_, err = testDB.Pool().Exec(ctx,
			`INSERT INTO skus (product_id, code, price_amount, status) VALUES ($1, $2, $3, $4)`,
			productId, f.skuCode, 1500, f.skuStatus)
		if err != nil {
			t.Fatalf("SKU INSERT失敗(%s):%v", f.skuCode, err)
		}
	}

	tests := []struct {
		testCase         string
		productName      string // 結果の中から探す商品
		wantHasActiveSku bool
	}{
		{"activeなSKUを持つ", "販売中タオル", true},
		{"全SKUが停止中", "全停止タオル", false},
		{"SKUを持たない", "SKUなしタオル", false},
	}

	input := uc.ProductListInput{Name: "タオル", Pagination: uc.Pagination{Page: 1, PerPage: 50}}
	for _, tt := range tests {
		t.Run(tt.testCase, func(t *testing.T) {
			items, _, err := repo.SearchProducts(context.Background(), input)
			if err != nil {
				t.Fatalf("予期しないエラー: %v", err)
			}
			p, found := findProduct(items, tt.productName)
			if !found {
				t.Fatalf("商品が結果に含まれていない: %s", tt.productName)
			}
			if p.HasActiveSku != tt.wantHasActiveSku {
				t.Errorf("hasActiveSku = %v, want %v", p.HasActiveSku, tt.wantHasActiveSku)
			}
		})
	}

}

func Test指定したカテゴリの商品を取得する(t *testing.T) {
	repo := NewProductRepository(testDB)

	ctx := context.Background()

	var clothingId string
	var shirtId string
	var poloId string
	var kitchenId string

	err := testDB.Pool().QueryRow(ctx, `INSERT INTO categories (name) VALUES ($1) RETURNING id`, "衣類").Scan(&clothingId)
	if err != nil {
		t.Fatalf("カテゴリ1 INSERT失敗:%v", err)
	}

	err = testDB.Pool().QueryRow(ctx, `INSERT INTO categories (name, parent_id) VALUES ($1, $2) RETURNING id`, "シャツ", clothingId).Scan(&shirtId)
	if err != nil {
		t.Fatalf("カテゴリ2 INSERT失敗:%v", err)
	}

	err = testDB.Pool().QueryRow(ctx, `INSERT INTO categories (name, parent_id) VALUES ($1, $2) RETURNING id`, "ポロシャツ", shirtId).Scan(&poloId)
	if err != nil {
		t.Fatalf("カテゴリ3 INSERT失敗:%v", err)
	}

	err = testDB.Pool().QueryRow(ctx,
		`INSERT INTO categories (name) VALUES ($1) RETURNING id`, "キッチン用品").Scan(&kitchenId)
	if err != nil {
		t.Fatalf("キッチンカテゴリ INSERT失敗(%s):%v", "キッチン用品", err)
	}

	_, err = testDB.Pool().Exec(ctx, `INSERT INTO products (name, category_id) VALUES ($1, $2)`,
		"Test服", clothingId)
	if err != nil {
		t.Fatalf("衣類 INSERT失敗(%s):%v", "Test服", err)
	}

	_, err = testDB.Pool().Exec(ctx, `INSERT INTO products (name, category_id) VALUES ($1, $2)`,
		"Testシャツ", shirtId)
	if err != nil {
		t.Fatalf("Testシャツ INSERT失敗(%s):%v", "Testシャツ", err)
	}

	_, err = testDB.Pool().Exec(ctx, `INSERT INTO products (name, category_id) VALUES ($1, $2)`,
		"TESTポロシャツ", poloId)
	if err != nil {
		t.Fatalf("TESTポロシャツ INSERT失敗(%s):%v", "TESTポロシャツ", err)
	}

	_, err = testDB.Pool().Exec(ctx,
		`INSERT INTO products (name, category_id) VALUES ($1, $2)`, "マグカップ", kitchenId)

	if err != nil {
		t.Fatalf("マグカップ INSERT失敗(%s):%v", "マグカップ", err)
	}

	_, err = testDB.Pool().Exec(ctx, `INSERT INTO products (name, category_id) VALUES ($1, $2)`,
		"リング", kitchenId)
	if err != nil {
		t.Fatalf("リング INSERT失敗(%s):%v", "リング", err)
	}

	_, err = testDB.Pool().Exec(ctx, `INSERT INTO products (name, category_id) VALUES ($1, $2)`,
		"雑貨", kitchenId)
	if err != nil {
		t.Fatalf("雑貨 INSERT失敗(%s):%v", "雑貨", err)
	}

	t.Cleanup(func() {
		testDB.Pool().Exec(ctx, `DELETE FROM products WHERE category_id in ($1, $2, $3)`, poloId, shirtId, clothingId)
	})

	wantHit := []string{"Test服", "Testシャツ", "TESTポロシャツ"}
	wantMiss := []string{"マグカップ", "リング", "雑貨"}
	input := uc.ProductListInput{CategoryID: clothingId, Pagination: uc.Pagination{Page: 1, PerPage: 50}}
	got, _, err := repo.SearchProducts(ctx, input)
	if err != nil {
		t.Fatalf("予期しないエラー: %v", err)
	}

	for _, hit := range wantHit {
		if !containsProduct(got, hit) {
			t.Errorf("ヒットすべき商品がない:%s", hit)
		}
	}

	for _, miss := range wantMiss {
		if containsProduct(got, miss) {
			t.Errorf("ヒットすべきではない商品:%s", miss)
		}
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

func findProduct(items []uc.ProductSummary, name string) (uc.ProductSummary, bool) {
	for _, item := range items {
		if item.Name == name {
			return item, true
		}
	}
	return uc.ProductSummary{}, false
}
