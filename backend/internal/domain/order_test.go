package domain

import (
	"errors"
	"testing"
	"time"
)

var testNow = time.Date(2026, 6, 1, 9, 30, 0, 0, time.UTC)

func testLines() []OrderLine {
	return []OrderLine{
		{SKUID: "sku-1", SKUCode: "MM-TOWEL-BL-M", Quantity: 2, UnitPrice: JPY(1480)},
		{SKUID: "sku-2", SKUCode: "MM-MUG-WH-L", Quantity: 1, UnitPrice: JPY(1980)},
	}
}

func newTestOrder(t *testing.T) *Order {
	t.Helper()
	o, err := NewOrder(
		"order-1", "MM-20260601-000123", "EC_OWN", "loc-1", "idem-key-1",
		CustomerInfo{Name: "山田 太郎", Email: "yamada@example.com", Phone: "090-1234-5678"},
		Address{PostalCode: "150-0001", Prefecture: "東京都", City: "渋谷区", Line1: "神宮前1-2-3"},
		testLines(), "actor-1", testNow,
	)
	if err != nil {
		t.Fatalf("NewOrder err = %v", err)
	}
	return o
}

func TestNewOrder(t *testing.T) {
	t.Run("作成直後はpendingで履歴が1件ある", func(t *testing.T) {
		o := newTestOrder(t)

		if o.Status != StatusPending {
			t.Errorf("Status = %q, want pending", o.Status)
		}
		// 作成そのものが追跡対象の変更なので、履歴が残っていなければならない(FR-309)。
		if len(o.History) != 1 {
			t.Fatalf("History の件数 = %d, want 1", len(o.History))
		}
		h := o.History[0]
		if h.From != "" || h.To != StatusPending || h.ChangedBy != "actor-1" {
			t.Errorf("History[0] = %+v, want {From:\"\" To:pending ChangedBy:actor-1}", h)
		}
	})

	t.Run("合計金額は明細から算出される", func(t *testing.T) {
		o := newTestOrder(t)

		// 1480*2 + 1980*1 = 4940
		if got := o.TotalAmount(); got.Amount != 4940 {
			t.Errorf("TotalAmount = %d, want 4940", got.Amount)
		}
	})

	t.Run("入力の不備は拒否される", func(t *testing.T) {
		tests := []struct {
			name    string
			mutate  func(*orderInput)
			wantErr error
		}{
			{"明細が空", func(in *orderInput) { in.lines = nil }, ErrValidation},
			{"チャネルコードなし", func(in *orderInput) { in.channelCode = "" }, ErrValidation},
			{"冪等キーなし", func(in *orderInput) { in.idempotencyKey = "" }, ErrValidation},
			{"顧客名なし", func(in *orderInput) { in.customer.Name = "" }, ErrValidation},
			{"数量0", func(in *orderInput) { in.lines[0].Quantity = 0 }, ErrValidation},
			{"数量が負", func(in *orderInput) { in.lines[0].Quantity = -1 }, ErrValidation},
			{"単価が負", func(in *orderInput) { in.lines[0].UnitPrice = JPY(-1) }, ErrValidation},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				in := defaultOrderInput()
				tt.mutate(&in)

				_, err := NewOrder(in.id, in.orderNumber, in.channelCode, in.locationID,
					in.idempotencyKey, in.customer, in.address, in.lines, in.actorID, testNow)

				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
			})
		}
	})
}

type orderInput struct {
	id, orderNumber, channelCode, locationID, idempotencyKey, actorID string
	customer                                                          CustomerInfo
	address                                                           Address
	lines                                                             []OrderLine
}

func defaultOrderInput() orderInput {
	return orderInput{
		id: "order-1", orderNumber: "MM-20260601-000123", channelCode: "EC_OWN",
		locationID: "loc-1", idempotencyKey: "idem-key-1", actorID: "actor-1",
		customer: CustomerInfo{Name: "山田 太郎"},
		address:  Address{PostalCode: "150-0001"},
		lines:    testLines(),
	}
}

// 状態遷移の全パターンを網羅する。許可された遷移だけが通り、
// それ以外はすべて ErrInvalidStatusTransition になることを確認する。
func TestOrder_状態遷移(t *testing.T) {
	all := []OrderStatus{StatusPending, StatusAllocated, StatusShipped, StatusCancelled}

	// docs/02-domain-model.md 4章の遷移図と一致させる。
	want := map[OrderStatus]map[OrderStatus]bool{
		StatusPending:   {StatusAllocated: true, StatusCancelled: true},
		StatusAllocated: {StatusShipped: true, StatusCancelled: true},
		StatusShipped:   {},
		StatusCancelled: {},
	}

	for _, from := range all {
		for _, to := range all {
			t.Run(string(from)+"→"+string(to), func(t *testing.T) {
				o := newTestOrder(t)
				o.Status = from

				got := o.CanTransitionTo(to)

				if got != want[from][to] {
					t.Errorf("CanTransitionTo(%s→%s) = %v, want %v", from, to, got, want[from][to])
				}
			})
		}
	}
}

func TestOrder_Allocate(t *testing.T) {
	o := newTestOrder(t)

	if err := o.Allocate("actor-2", testNow.Add(time.Hour)); err != nil {
		t.Fatalf("Allocate err = %v", err)
	}

	if o.Status != StatusAllocated {
		t.Errorf("Status = %q, want allocated", o.Status)
	}
	if len(o.History) != 2 {
		t.Fatalf("History の件数 = %d, want 2", len(o.History))
	}
	h := o.History[1]
	if h.From != StatusPending || h.To != StatusAllocated || h.ChangedBy != "actor-2" {
		t.Errorf("History[1] = %+v", h)
	}
}

func TestOrder_Cancel(t *testing.T) {
	t.Run("理由を添えてキャンセルできる", func(t *testing.T) {
		o := newTestOrder(t)

		if err := o.Cancel("actor-2", "顧客都合", testNow.Add(time.Hour)); err != nil {
			t.Fatalf("Cancel err = %v", err)
		}

		if o.Status != StatusCancelled {
			t.Errorf("Status = %q, want cancelled", o.Status)
		}
		if o.History[1].Reason != "顧客都合" {
			t.Errorf("Reason = %q, want 顧客都合", o.History[1].Reason)
		}
	})

	t.Run("理由なしのキャンセルは拒否される", func(t *testing.T) {
		o := newTestOrder(t)

		err := o.Cancel("actor-2", "", testNow)

		if !errors.Is(err, ErrValidation) {
			t.Fatalf("err = %v, ErrValidation を期待", err)
		}
		if o.Status != StatusPending {
			t.Errorf("失敗したキャンセルが状態を変更している: %q", o.Status)
		}
	})

	t.Run("出荷済の注文はキャンセルできない", func(t *testing.T) {
		o := newTestOrder(t)
		o.Status = StatusShipped

		err := o.Cancel("actor-2", "顧客都合", testNow)

		if !errors.Is(err, ErrInvalidStatusTransition) {
			t.Fatalf("err = %v, ErrInvalidStatusTransition を期待", err)
		}
		if o.Status != StatusShipped {
			t.Errorf("Status = %q, want shipped", o.Status)
		}
	})
}

// 受付から出荷までを通しでたどり、履歴が漏れなく積み上がることを確認する。
func TestOrder_受付から出荷完了までの通し(t *testing.T) {
	o := newTestOrder(t)

	if err := o.Allocate("op-1", testNow.Add(1*time.Hour)); err != nil {
		t.Fatalf("Allocate err = %v", err)
	}
	if err := o.Ship("op-1", testNow.Add(5*time.Hour)); err != nil {
		t.Fatalf("Ship err = %v", err)
	}

	if o.Status != StatusShipped {
		t.Fatalf("Status = %q, want shipped", o.Status)
	}

	// 受付 → 出荷指示 → 出荷完了 の3件
	wantTransitions := []struct{ from, to OrderStatus }{
		{"", StatusPending},
		{StatusPending, StatusAllocated},
		{StatusAllocated, StatusShipped},
	}
	if len(o.History) != len(wantTransitions) {
		t.Fatalf("History の件数 = %d, want %d", len(o.History), len(wantTransitions))
	}
	for i, w := range wantTransitions {
		if o.History[i].From != w.from || o.History[i].To != w.to {
			t.Errorf("History[%d] = %s→%s, want %s→%s",
				i, o.History[i].From, o.History[i].To, w.from, w.to)
		}
	}

	// 終端状態なので、ここから先へは進めない。
	if err := o.Cancel("op-1", "誤操作", testNow.Add(6*time.Hour)); !errors.Is(err, ErrInvalidStatusTransition) {
		t.Errorf("出荷済からのキャンセル err = %v, ErrInvalidStatusTransition を期待", err)
	}
}
