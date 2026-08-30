package domain

import (
	"slices"
	"time"
)

// OrderStatus は注文の状態。遷移可能な経路は docs/02-domain-model.md 4章の図を参照。
type OrderStatus string

const (
	StatusPending   OrderStatus = "pending"   // 受付済。在庫は引当済
	StatusAllocated OrderStatus = "allocated" // 出荷指示済。WMSへ連携済
	StatusShipped   OrderStatus = "shipped"   // 出荷完了。出庫確定済
	StatusCancelled OrderStatus = "cancelled" // キャンセル。引当解除済
)

// allowedTransitions は「この状態から遷移できる先」の定義。
// 状態遷移のルールをコード各所のif文に散らさず、この1箇所に集約する。
//
// shipped と cancelled は終端状態で、遷移先を持たない。
// 返品はフェーズ1のスコープ外で、入荷登録(FR-205)として扱う。
var allowedTransitions = map[OrderStatus][]OrderStatus{
	StatusPending:   {StatusAllocated, StatusCancelled},
	StatusAllocated: {StatusShipped, StatusCancelled},
	StatusShipped:   {},
	StatusCancelled: {},
}

// Money は金額。日本円のみを扱うため Amount は「円」の整数。
// 浮動小数点を使わないので丸め誤差が発生しない（docs/03-erd.md 2.1）。
type Money struct {
	Amount   int64
	Currency string
}

func JPY(amount int64) Money { return Money{Amount: amount, Currency: "JPY"} }

// CustomerInfo と Address は個人情報を含む。ログに出力してはならない(NFR-06)。
type CustomerInfo struct {
	Name  string
	Email string
	Phone string
}

type Address struct {
	PostalCode string
	Prefecture string
	City       string
	Line1      string
	Line2      string
}

// OrderLine は注文明細。SKUCode と UnitPrice は注文時点の値のコピーを保持する。
// マスタの価格が改定されても過去の注文金額が変わらないようにするため（docs/03-erd.md 2.5）。
type OrderLine struct {
	SKUID     string
	SKUCode   string
	Quantity  int
	UnitPrice Money
}

func (l OrderLine) Subtotal() Money {
	return Money{Amount: l.UnitPrice.Amount * int64(l.Quantity), Currency: l.UnitPrice.Currency}
}

// StatusChange は状態遷移の記録1件。追記のみ(FR-309)。
type StatusChange struct {
	From      OrderStatus // 注文作成時は空文字
	To        OrderStatus
	ChangedBy string
	ChangedAt time.Time
	Reason    string
}

// Order は注文の集約ルート。
type Order struct {
	ID              string
	OrderNumber     string
	ChannelCode     string
	LocationID      string
	Status          OrderStatus
	Customer        CustomerInfo
	ShippingAddress Address
	IdempotencyKey  string
	Lines           []OrderLine
	PlacedAt        time.Time
	History         []StatusChange
}

// NewOrder は注文を組み立てる。作成された時点で status は pending であり、
// 「在庫の引当が済んでいること」を前提とする。引当自体はユースケース層が
// InventoryItem.Reserve を呼んで行い、同一トランザクションでこの注文を保存する。
func NewOrder(
	id, orderNumber, channelCode, locationID, idempotencyKey string,
	customer CustomerInfo,
	address Address,
	lines []OrderLine,
	actorID string,
	now time.Time,
) (*Order, error) {
	if len(lines) == 0 {
		return nil, newValidationError("lines", "注文明細は1件以上必要です")
	}
	if channelCode == "" {
		return nil, newValidationError("channelCode", "チャネルコードは必須です")
	}
	if idempotencyKey == "" {
		// 勝手に生成しない。生成すると再送が別注文になり冪等性が失われる（ADR-0003）。
		return nil, newValidationError("idempotencyKey", "冪等キーは必須です")
	}
	if customer.Name == "" {
		return nil, newValidationError("customer.name", "顧客名は必須です")
	}
	for _, l := range lines {
		if l.Quantity <= 0 {
			return nil, newValidationError("lines", "数量は1以上である必要があります")
		}
		if l.UnitPrice.Amount < 0 {
			return nil, newValidationError("lines", "単価が不正です")
		}
	}

	return &Order{
		ID:              id,
		OrderNumber:     orderNumber,
		ChannelCode:     channelCode,
		LocationID:      locationID,
		Status:          StatusPending,
		Customer:        customer,
		ShippingAddress: address,
		IdempotencyKey:  idempotencyKey,
		Lines:           lines,
		PlacedAt:        now,
		History: []StatusChange{{
			From: "", To: StatusPending, ChangedBy: actorID, ChangedAt: now, Reason: "注文受付",
		}},
	}, nil
}

// TotalAmount は明細の小計合計。orders.total_amount に保存するが、
// 常にこのメソッドで算出し、外部から任意の金額を渡せないようにする。
func (o *Order) TotalAmount() Money {
	var total int64
	for _, l := range o.Lines {
		total += l.Subtotal().Amount
	}
	return JPY(total)
}

// CanTransitionTo は遷移可否だけを判定する。副作用はない。
func (o *Order) CanTransitionTo(to OrderStatus) bool {
	return slices.Contains(allowedTransitions[o.Status], to)
}

// transitionTo は状態を変更し、履歴を1件追加する。
// すべての状態変更はこのメソッドを通す。直接 o.Status に代入してはならない。
// 履歴の記録漏れ(R-02違反)を構造的に防ぐため。
func (o *Order) transitionTo(to OrderStatus, actorID, reason string, now time.Time) error {
	if !o.CanTransitionTo(to) {
		return &StatusTransitionError{From: o.Status, To: to}
	}

	from := o.Status
	o.Status = to
	o.History = append(o.History, StatusChange{
		From: from, To: to, ChangedBy: actorID, ChangedAt: now, Reason: reason,
	})
	return nil
}

// Allocate は出荷指示(FR-305)。pending からのみ可能。
func (o *Order) Allocate(actorID string, now time.Time) error {
	return o.transitionTo(StatusAllocated, actorID, "出荷指示", now)
}

// Ship は出荷完了(FR-306)。allocated からのみ可能。
// 呼び出し側は同一トランザクションで InventoryItem.Ship による出庫確定を行うこと。
func (o *Order) Ship(actorID string, now time.Time) error {
	return o.transitionTo(StatusShipped, actorID, "出荷完了", now)
}

// Cancel はキャンセル(FR-307)。理由を必須にする。
// 呼び出し側は同一トランザクションで InventoryItem.Release による引当解除を行うこと。
func (o *Order) Cancel(actorID, reason string, now time.Time) error {
	if reason == "" {
		return newValidationError("reason", "キャンセル理由は必須です")
	}
	return o.transitionTo(StatusCancelled, actorID, reason, now)
}

// StatusTransitionError は許可されていない状態遷移。
// 「出荷済の注文をキャンセルしようとした」など、業務上ありうる操作ミスを表す。
type StatusTransitionError struct {
	From OrderStatus
	To   OrderStatus
}

func (e *StatusTransitionError) Error() string {
	return "cannot transition from " + string(e.From) + " to " + string(e.To)
}

func (e *StatusTransitionError) Unwrap() error { return ErrInvalidStatusTransition }
