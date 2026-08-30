package domain

// InventoryItem は「SKU × 拠点」ごとの在庫。この集約が売り越し防止(R-01)の責任を持つ。
//
// 不変条件（docs/02-domain-model.md 3章）:
//  1. OnHand >= 0
//  2. Reserved >= 0
//  3. Reserved <= OnHand
//
// 用語は docs/02-domain-model.md 6章のユビキタス言語に従う。
// OnHand=実在庫数, Reserved=引当済数, Available=引当可能数。
type InventoryItem struct {
	ID         string
	SKUID      string
	SKUCode    string
	LocationID string
	OnHand     int
	Reserved   int
	Version    int
}

// MovementType は在庫がどう動いたかの種別。inventory_movements に記録する。
type MovementType string

const (
	MovementReserve MovementType = "reserve" // 引当（注文確定）
	MovementRelease MovementType = "release" // 引当解除（キャンセル）
	MovementShip    MovementType = "ship"    // 出庫確定（出荷完了）
	MovementReceive MovementType = "receive" // 入荷
	MovementAdjust  MovementType = "adjust"  // 棚卸補正
)

// Movement は在庫変動の記録1件。追記のみで、更新も削除もしない(FR-207)。
// Delta は「変化量」であり変化後の値ではない。履歴を積み上げれば現在値になる。
type Movement struct {
	Type          MovementType
	OnHandDelta   int
	ReservedDelta int
	ReasonCode    string
	Note          string
}

// Available は引当可能数を返す。新規注文が引き当てられる数量。
func (i *InventoryItem) Available() int {
	return i.OnHand - i.Reserved
}

// Reserve は注文確定時に在庫を引き当てる。OnHand は動かさず Reserved だけ増やす。
// 物理的な在庫はまだ倉庫にあり、出荷されるまで減らないため。
//
// 引当可能数が足りなければ ErrInsufficientStock を返し、状態を変更しない。
// ここが売り越しを防ぐ唯一の判断点。
func (i *InventoryItem) Reserve(qty int) (Movement, error) {
	if qty <= 0 {
		return Movement{}, newValidationError("quantity", "引当数は1以上である必要があります")
	}
	if i.Available() < qty {
		return Movement{}, &InsufficientStockError{
			Shortages: []StockShortage{{
				SKUCode:   i.SKUCode,
				Requested: qty,
				Available: i.Available(),
			}},
		}
	}

	i.Reserved += qty
	return Movement{Type: MovementReserve, ReservedDelta: qty}, nil
}

// Release は注文キャンセル時に引当を戻す。Reserved を減らすだけで OnHand は変わらない。
func (i *InventoryItem) Release(qty int) (Movement, error) {
	if qty <= 0 {
		return Movement{}, newValidationError("quantity", "解除数は1以上である必要があります")
	}
	// 引当済を超えて解除すると Reserved が負になり、不変条件2を破る。
	if i.Reserved < qty {
		return Movement{}, newValidationError("quantity", "引当済数を超えて解除することはできません")
	}

	i.Reserved -= qty
	return Movement{Type: MovementRelease, ReservedDelta: -qty}, nil
}

// Ship は出荷完了時の出庫確定。実際に商品が倉庫を出るので、
// OnHand と Reserved の両方を減らす。Available は変化しない
// （引当済のぶんは、もともと Available に含まれていないため）。
func (i *InventoryItem) Ship(qty int) (Movement, error) {
	if qty <= 0 {
		return Movement{}, newValidationError("quantity", "出庫数は1以上である必要があります")
	}
	// 引当されていない在庫は出荷できない。注文を経ない出庫を防ぐ。
	if i.Reserved < qty {
		return Movement{}, newValidationError("quantity", "引当済数を超えて出庫することはできません")
	}

	i.OnHand -= qty
	i.Reserved -= qty
	return Movement{Type: MovementShip, OnHandDelta: -qty, ReservedDelta: -qty}, nil
}

// Receive は仕入や返品による入荷。OnHand だけが増える。
func (i *InventoryItem) Receive(qty int, reasonCode string) (Movement, error) {
	if qty <= 0 {
		return Movement{}, newValidationError("quantity", "入荷数は1以上である必要があります")
	}
	if reasonCode == "" {
		return Movement{}, newValidationError("reasonCode", "入荷理由は必須です")
	}

	i.OnHand += qty
	return Movement{Type: MovementReceive, OnHandDelta: qty, ReasonCode: reasonCode}, nil
}

// Adjust は棚卸差異の補正。delta は増減どちらもありうる。
// 追跡可能性(R-02)のため理由コードとメモを必須にする。実行できるのは管理者のみ(FR-206)だが、
// 権限判定はドメイン層の責務ではないのでユースケース層で行う。
func (i *InventoryItem) Adjust(delta int, reasonCode, note string) (Movement, error) {
	if delta == 0 {
		return Movement{}, newValidationError("delta", "補正量に0は指定できません")
	}
	if reasonCode == "" {
		return Movement{}, newValidationError("reasonCode", "補正理由コードは必須です")
	}
	if note == "" {
		return Movement{}, newValidationError("note", "補正メモは必須です")
	}

	after := i.OnHand + delta
	if after < 0 {
		return Movement{}, newValidationError("delta", "補正後の実在庫数が負になります")
	}
	// 減らす方向の補正で、引当済を下回ると不変条件3が破れる。
	// 例: OnHand=10, Reserved=8 のときに -5 すると OnHand=5 < Reserved=8 となり、
	// 引当済の商品が物理的に存在しないことになってしまう。
	if after < i.Reserved {
		return Movement{}, newValidationError("delta",
			"補正後の実在庫数が引当済数を下回ります。先に該当注文のキャンセルが必要です")
	}

	i.OnHand = after
	return Movement{Type: MovementAdjust, OnHandDelta: delta, ReasonCode: reasonCode, Note: note}, nil
}
