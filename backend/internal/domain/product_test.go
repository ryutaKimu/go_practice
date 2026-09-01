package domain

import "testing"

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
