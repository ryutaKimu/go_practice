package domain

import (
	"errors"
	"testing"
)

// newItem は「実在庫10・引当0」のテスト用在庫を作る。
// docs/02-domain-model.md 3章の具体例と同じ初期状態。
func newItem(onHand, reserved int) *InventoryItem {
	return &InventoryItem{
		ID:         "inv-1",
		SKUID:      "sku-1",
		SKUCode:    "MM-TOWEL-BL-M",
		LocationID: "loc-1",
		OnHand:     onHand,
		Reserved:   reserved,
	}
}

// assertInvariants は3つの不変条件がすべて保たれていることを確認する。
// どの操作のあとでも成り立つべきものなので、各テストの最後に必ず呼ぶ。
func assertInvariants(t *testing.T, i *InventoryItem) {
	t.Helper()
	if i.OnHand < 0 {
		t.Errorf("不変条件1違反: OnHand=%d が負です", i.OnHand)
	}
	if i.Reserved < 0 {
		t.Errorf("不変条件2違反: Reserved=%d が負です", i.Reserved)
	}
	if i.Reserved > i.OnHand {
		t.Errorf("不変条件3違反: Reserved=%d > OnHand=%d（売り越し状態）", i.Reserved, i.OnHand)
	}
}

func TestAvailable(t *testing.T) {
	if got := newItem(10, 3).Available(); got != 7 {
		t.Errorf("Available() = %d, want 7", got)
	}
}

func TestReserve(t *testing.T) {
	tests := []struct {
		name         string
		onHand       int
		reserved     int
		qty          int
		wantErr      error
		wantReserved int
	}{
		{"引当可能数の範囲内", 10, 0, 3, nil, 3},
		{"引当可能数ちょうど", 10, 0, 10, nil, 10},
		{"既存の引当がある状態で追加", 10, 3, 7, nil, 10},
		{"引当可能数を1つ超える", 10, 3, 8, ErrInsufficientStock, 3},
		{"実在庫はあるが全て引当済", 10, 10, 1, ErrInsufficientStock, 10},
		{"数量が0", 10, 0, 0, ErrValidation, 0},
		{"数量が負", 10, 0, -1, ErrValidation, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := newItem(tt.onHand, tt.reserved)
			mv, err := item.Reserve(tt.qty)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Reserve(%d) err = %v, want %v", tt.qty, err, tt.wantErr)
			}
			if item.Reserved != tt.wantReserved {
				t.Errorf("Reserved = %d, want %d", item.Reserved, tt.wantReserved)
			}
			// 引当では実在庫は動かない。商品はまだ倉庫にあるため。
			if item.OnHand != tt.onHand {
				t.Errorf("OnHand = %d, want %d（引当でOnHandは変化しない）", item.OnHand, tt.onHand)
			}
			if tt.wantErr == nil && mv.ReservedDelta != tt.qty {
				t.Errorf("Movement.ReservedDelta = %d, want %d", mv.ReservedDelta, tt.qty)
			}
			assertInvariants(t, item)
		})
	}
}

// 在庫不足のエラーには、何がいくつ足りないかの内訳が入っている必要がある。
// docs/04-api-spec.md 3章のレスポンス例を満たすため。
func TestReserve_不足時のエラーに内訳が含まれる(t *testing.T) {
	item := newItem(10, 3)

	_, err := item.Reserve(8)

	var stockErr *InsufficientStockError
	if !errors.As(err, &stockErr) {
		t.Fatalf("err = %v, InsufficientStockError を期待", err)
	}
	if len(stockErr.Shortages) != 1 {
		t.Fatalf("Shortages の件数 = %d, want 1", len(stockErr.Shortages))
	}
	s := stockErr.Shortages[0]
	if s.SKUCode != "MM-TOWEL-BL-M" || s.Requested != 8 || s.Available != 7 {
		t.Errorf("Shortage = %+v, want {MM-TOWEL-BL-M 8 7}", s)
	}
}

func TestRelease(t *testing.T) {
	tests := []struct {
		name         string
		onHand       int
		reserved     int
		qty          int
		wantErr      error
		wantReserved int
	}{
		{"引当の一部を解除", 10, 5, 3, nil, 2},
		{"引当を全て解除", 10, 5, 5, nil, 0},
		{"引当済を超えて解除", 10, 5, 6, ErrValidation, 5},
		{"引当がない状態で解除", 10, 0, 1, ErrValidation, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := newItem(tt.onHand, tt.reserved)
			_, err := item.Release(tt.qty)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Release(%d) err = %v, want %v", tt.qty, err, tt.wantErr)
			}
			if item.Reserved != tt.wantReserved {
				t.Errorf("Reserved = %d, want %d", item.Reserved, tt.wantReserved)
			}
			if item.OnHand != tt.onHand {
				t.Errorf("OnHand = %d, want %d（解除でOnHandは変化しない）", item.OnHand, tt.onHand)
			}
			assertInvariants(t, item)
		})
	}
}

func TestShip(t *testing.T) {
	t.Run("引当済のぶんを出庫すると実在庫と引当の両方が減る", func(t *testing.T) {
		item := newItem(10, 3)

		mv, err := item.Ship(3)
		if err != nil {
			t.Fatalf("Ship(3) err = %v", err)
		}

		if item.OnHand != 7 {
			t.Errorf("OnHand = %d, want 7", item.OnHand)
		}
		if item.Reserved != 0 {
			t.Errorf("Reserved = %d, want 0", item.Reserved)
		}
		// 引当済のぶんはもともと Available に含まれないので、出庫しても Available は変わらない。
		if item.Available() != 7 {
			t.Errorf("Available = %d, want 7（出庫でAvailableは変化しない）", item.Available())
		}
		if mv.OnHandDelta != -3 || mv.ReservedDelta != -3 {
			t.Errorf("Movement = %+v, want OnHandDelta=-3 ReservedDelta=-3", mv)
		}
		assertInvariants(t, item)
	})

	t.Run("引当されていない在庫は出庫できない", func(t *testing.T) {
		item := newItem(10, 0)

		_, err := item.Ship(1)

		if !errors.Is(err, ErrValidation) {
			t.Fatalf("err = %v, ErrValidation を期待（注文を経ない出庫は許可しない）", err)
		}
		assertInvariants(t, item)
	})
}

func TestReceive(t *testing.T) {
	t.Run("入荷で実在庫だけが増える", func(t *testing.T) {
		item := newItem(10, 3)

		mv, err := item.Receive(5, "PURCHASE")
		if err != nil {
			t.Fatalf("Receive err = %v", err)
		}

		if item.OnHand != 15 {
			t.Errorf("OnHand = %d, want 15", item.OnHand)
		}
		if item.Reserved != 3 {
			t.Errorf("Reserved = %d, want 3（入荷でReservedは変化しない）", item.Reserved)
		}
		if mv.ReasonCode != "PURCHASE" {
			t.Errorf("ReasonCode = %q, want PURCHASE", mv.ReasonCode)
		}
		assertInvariants(t, item)
	})

	t.Run("理由コードなしは拒否される", func(t *testing.T) {
		item := newItem(10, 0)
		if _, err := item.Receive(5, ""); !errors.Is(err, ErrValidation) {
			t.Fatalf("err = %v, ErrValidation を期待", err)
		}
	})
}

func TestInventory_入荷した分は引当できる(t *testing.T) {
	item := newItem(0, 0)
	mv, err := item.Receive(20, "PURCHASE")
	if err != nil {
		t.Fatalf("Receive err = %v", err)
	}
	if mv.ReasonCode != "PURCHASE" {
		t.Errorf("ReasonCode = %q, want PURCHASE", mv.ReasonCode)
	}

	if _, err := item.Reserve(5); err != nil {
		t.Fatalf("注文の引当に失敗: %v", err)
	}

	if item.OnHand != 20 || item.Reserved != 5 || item.Available() != 15 {
		t.Fatalf("予約後 = (%d, %d, %d), want (20, 5, 15)", item.OnHand, item.Reserved, item.Available())
	}
	assertInvariants(t, item)
}

func TestAdjust(t *testing.T) {
	tests := []struct {
		name       string
		onHand     int
		reserved   int
		delta      int
		reasonCode string
		note       string
		wantErr    error
		wantOnHand int
	}{
		{"棚卸で不足が判明", 10, 0, -1, "STOCKTAKING_LOSS", "棚卸差異", nil, 9},
		{"棚卸で超過が判明", 10, 0, 2, "STOCKTAKING_GAIN", "棚卸差異", nil, 12},
		{"引当済ちょうどまで減らす", 10, 5, -5, "STOCKTAKING_LOSS", "棚卸差異", nil, 5},
		{"引当済を下回る補正", 10, 5, -6, "STOCKTAKING_LOSS", "棚卸差異", ErrValidation, 10},
		{"補正後に実在庫が負", 10, 0, -11, "STOCKTAKING_LOSS", "棚卸差異", ErrValidation, 10},
		{"補正量0", 10, 0, 0, "STOCKTAKING_LOSS", "棚卸差異", ErrValidation, 10},
		{"理由コードなし", 10, 0, -1, "", "棚卸差異", ErrValidation, 10},
		{"メモなし", 10, 0, -1, "STOCKTAKING_LOSS", "", ErrValidation, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := newItem(tt.onHand, tt.reserved)
			_, err := item.Adjust(tt.delta, tt.reasonCode, tt.note)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Adjust(%d) err = %v, want %v", tt.delta, err, tt.wantErr)
			}
			if item.OnHand != tt.wantOnHand {
				t.Errorf("OnHand = %d, want %d", item.OnHand, tt.wantOnHand)
			}
			assertInvariants(t, item)
		})
	}
}

// docs/02-domain-model.md 3章の具体例をそのままなぞる。
// 設計ドキュメントに書いた挙動と実装が一致していることを保証する。
func TestInventory_設計ドキュメントのシナリオ(t *testing.T) {
	item := newItem(10, 0)

	// 注文A(3個)を引当
	if _, err := item.Reserve(3); err != nil {
		t.Fatalf("注文Aの引当に失敗: %v", err)
	}
	if item.OnHand != 10 || item.Reserved != 3 || item.Available() != 7 {
		t.Fatalf("注文A引当後 = (%d, %d, %d), want (10, 3, 7)", item.OnHand, item.Reserved, item.Available())
	}

	// 注文B(8個)は引当可能数7を超えるので失敗する ★売り越しを防ぐ地点
	if _, err := item.Reserve(8); !errors.Is(err, ErrInsufficientStock) {
		t.Fatalf("注文Bは在庫不足で失敗すべき: err = %v", err)
	}
	if item.Reserved != 3 {
		t.Fatalf("失敗した引当が状態を変更している: Reserved = %d, want 3", item.Reserved)
	}

	// 注文Aが出荷完了
	if _, err := item.Ship(3); err != nil {
		t.Fatalf("出庫確定に失敗: %v", err)
	}
	if item.OnHand != 7 || item.Reserved != 0 || item.Available() != 7 {
		t.Fatalf("出荷後 = (%d, %d, %d), want (7, 0, 7)", item.OnHand, item.Reserved, item.Available())
	}

	// 棚卸で1個不足が判明
	if _, err := item.Adjust(-1, "STOCKTAKING_LOSS", "棚卸差異"); err != nil {
		t.Fatalf("補正に失敗: %v", err)
	}
	if item.OnHand != 6 || item.Available() != 6 {
		t.Fatalf("補正後 = (%d, %d), want (6, 6)", item.OnHand, item.Available())
	}

	assertInvariants(t, item)
}
