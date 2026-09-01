package domain

import (
	"errors"
	"testing"
)

// newTestProduct はテスト用の商品データを作成する
func newTestProduct(ps ProductStatus, ss SKUStatus) *Product {
	return &Product{
		ID:          "product-1",
		Name:        "シャツ",
		Description: "テストシャツです",
		CategoryID:  "cat-1",
		Status:      ps,
		SKUs: []SKU{
			{ID: "sku-1", Code: "MM-SHIRT-BL-M", Status: ss},
			{ID: "sku-2", Code: "MM-SHIRT-BL-L", Status: SKUStatusActive},
		},
	}
}

func TestNewProduct_SKUが1つ以上存在していること(t *testing.T) {
	_, err := NewProduct("product-1", "Y-シャツ", "テストシャツです", "car-1", nil)
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
}

func TestNewProduct_商品名が空なら作成不可(t *testing.T) {
	skus := []SKU{{ID: "sku-1", Code: "MM-SHIRT-BL-M", Status: SKUStatusActive}}
	_, err := NewProduct("product-1", "", "テストシャツです", "car-1", skus)
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
}

func TestNewProduct_作成直後は販売中(t *testing.T) {
	skus := []SKU{{ID: "sku-1", Code: "MM-SHIRT-BL-M", Status: SKUStatusActive}}
	p, err := NewProduct("product-1", "Y-シャツ", "テストシャツです", "car-1", skus)
	if err != nil {
		t.Fatalf("NewProduct() err = %v, want nil", err)
	}
	if p.Status != ProductStatusActive {
		t.Errorf("Status = %s, want %s", p.Status, ProductStatusActive)
	}
	if !p.CanOrder("MM-SHIRT-BL-M") {
		t.Error("作成直後の商品が注文不可になっている")
	}
}

func TestNewProduct_skuステータスが未設定ならデフォルトでactiveになる(t *testing.T) {
	skus := []SKU{{ID: "sku-1", Code: "MM-SHIRT-BL-M", Status: ""}}
	p, err := NewProduct("product-1", "Y-シャツ", "テストシャツです", "car-1", skus)
	if err != nil {
		t.Fatalf("NewProduct() err = %v, want nil", err)
	}
	if !p.CanOrder("MM-SHIRT-BL-M") {
		t.Error("ステータス未設定のSKUが注文不可になっている")
	}
}

func TestNewProduct_skuコードが空なら作成できない(t *testing.T) {
	skus := []SKU{{ID: "sku-1", Code: "", Status: ""}}
	_, err := NewProduct("product-1", "Y-シャツ", "テストシャツです", "car-1", skus)

	if !errors.Is(err, ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
}

// Suspend は CanOrder と組で意味を持つため、結合して確認する。
func TestSuspend_販売停止後は注文できない(t *testing.T) {
	p := newTestProduct(ProductStatusActive, SKUStatusActive)
	if !p.CanOrder("MM-SHIRT-BL-M") {
		t.Fatal("前提条件が成立していない（停止前に注文できていない）")
	}

	p.Suspend()

	if p.Status != ProductStatusSuspended {
		t.Errorf("Status = %s, want %s", p.Status, ProductStatusSuspended)
	}
	// SKU 自体は active のままだが、商品停止によって注文不可になる
	if p.CanOrder("MM-SHIRT-BL-M") {
		t.Error("販売停止後も注文可能になっている")
	}
}

func TestProduct_注文可能かの判定(t *testing.T) {
	tests := []struct {
		name          string
		productStatus ProductStatus
		skuStatus     SKUStatus
		skuCode       string
		want          bool
	}{
		{"商品もSKUも販売中", ProductStatusActive, SKUStatusActive, "MM-SHIRT-BL-M", true},
		{"商品のみ販売中", ProductStatusActive, SKUStatusSuspended, "MM-SHIRT-BL-M", false},
		{"SKUのみ販売中", ProductStatusSuspended, SKUStatusActive, "MM-SHIRT-BL-M", false},
		{"両方販売停止", ProductStatusSuspended, SKUStatusSuspended, "MM-SHIRT-BL-M", false},
		{"指定したコードのSKUを見ている", ProductStatusActive, SKUStatusSuspended, "MM-SHIRT-BL-L", true},
		{"存在しないSKUコード", ProductStatusActive, SKUStatusActive, "MM-NOTHING", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			product := newTestProduct(tt.productStatus, tt.skuStatus)
			got := product.CanOrder(tt.skuCode)
			if got != tt.want {
				t.Errorf("CanOrder(%q) = %v, want %v (product=%s, sku=%s)",
					tt.skuCode, got, tt.want, tt.productStatus, tt.skuStatus)
			}
		})
	}
}
