package domain

import "slices"

type SKUStatus string
type ProductStatus string

const (
	SKUStatusActive    SKUStatus = "active"
	SKUStatusSuspended SKUStatus = "suspended"
)

const (
	ProductStatusActive    ProductStatus = "active"
	ProductStatusSuspended ProductStatus = "suspended"
)

// SKU は商品のバリエーション（色・サイズ違い）。在庫と価格はSKU単位で持つ。
type SKU struct {
	ID         string
	Code       string
	Price      Money
	Attributes map[string]string
	Status     SKUStatus
}

// Product は商品。SKUを1件以上持つ。0件は不可
// Productのステータスをsuspendedにすると、
// 配下のSKU全てが販売停止になる。
// SKU単体での販売停止も可能
// 詳細は docs/02-domain-model.md 2章。
type Product struct {
	ID          string
	Name        string
	Description string
	CategoryID  string
	Status      ProductStatus
	SKUs        []SKU
}

func NewProduct(id, name, description, categoryID string,
	skus []SKU,
) (*Product, error) {
	if len(skus) == 0 {
		return nil, newValidationError("skus", "SKUは1件以上必要です")
	}

	if name == "" {
		return nil, newValidationError("name", "商品名が空白です")
	}

	// 呼び出し側のスライスをそのまま保持すると、生成後に外から要素を
	// 書き換えられ、ここでの検査を迂回できてしまうため複製する。
	skus = slices.Clone(skus)

	// SKUコードは全体で一意（docs/02-domain-model.md 2章、skus.code の UNIQUE 制約）。
	// 同一商品内の重複だけでもここで弾かないと、CanOrder が
	// スライスの順序次第で違う答えを返す。
	seen := make(map[string]struct{}, len(skus))

	for i, s := range skus {
		// 空コードを許すと CanOrder("") が意図しないSKUに当たる
		if s.Code == "" {
			return nil, newValidationError("skus", "SKUコードは必須です")
		}
		if _, ok := seen[s.Code]; ok {
			return nil, newValidationError("skus", "SKUコードが重複しています: "+s.Code)
		}
		seen[s.Code] = struct{}{}

		// skus.price_amount の CHECK (price_amount >= 0) に対応
		if s.Price.Amount < 0 {
			return nil, newValidationError("skus", "価格は0以上である必要があります")
		}

		// SKUStatus のゼロ値 "" のままだと CanOrder が常に false になる。
		// 呼び出し側に必須指定させる案もあったが、skus.status の
		// DEFAULT 'active'（migrations/000001_init_schema.up.sql）に揃えて
		// DBとドメインで同じ既定値を持つ形にした。
		if s.Status == "" {
			skus[i].Status = SKUStatusActive
		}
	}

	return &Product{
		ID:          id,
		Name:        name,
		Description: description,
		CategoryID:  categoryID,
		Status:      ProductStatusActive,
		SKUs:        skus,
	}, nil
}

// Suspend は商品を販売停止にする。配下SKUの status は書き換えず、
// 商品停止の波及は CanOrder の判定で表現する。SKU側を一括更新すると、
// 停止前にSKU単体で止めていたものと区別がつかなくなるため
// （docs/02-domain-model.md 2章）。
func (p *Product) Suspend() {
	p.Status = ProductStatusSuspended
}

// CanOrder は指定コードのSKUが注文可能かを返す。
// Product と SKU の両方が active のときだけ true。
// 詳細は docs/02-domain-model.md 2章。
func (p *Product) CanOrder(skuCode string) bool {
	if p.Status != ProductStatusActive {
		return false
	}
	for _, s := range p.SKUs {
		if s.Code == skuCode {
			return s.Status == SKUStatusActive
		}
	}
	return false
}
