package domain

import (
	"errors"
	"fmt"
)

// ドメイン層が返すエラーの種類。HTTP層はこれを見てステータスコードを決める。
// 対応表は docs/05-architecture.md 5章。
var (
	ErrValidation              = errors.New("validation error")
	ErrNotFound                = errors.New("not found")
	ErrInsufficientStock       = errors.New("insufficient stock")
	ErrInvalidStatusTransition = errors.New("invalid status transition")
	ErrForbidden               = errors.New("forbidden")
)

// StockShortage は在庫が足りなかったSKU1件分の情報。
// 注文登録で複数SKUが不足した場合、すべてを集めてクライアントに返す
// （docs/04-api-spec.md 3章）。
type StockShortage struct {
	SKUCode   string
	Requested int
	Available int
}

// InsufficientStockError は ErrInsufficientStock に不足の内訳を添えたもの。
type InsufficientStockError struct {
	Shortages []StockShortage
}

func (e *InsufficientStockError) Error() string {
	return fmt.Sprintf("insufficient stock for %d sku(s)", len(e.Shortages))
}

func (e *InsufficientStockError) Unwrap() error { return ErrInsufficientStock }

// ValidationError は入力値の不備。Field はクライアントへ返すため、
// 内部のカラム名ではなくAPIのフィールド名を入れる。
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

func (e *ValidationError) Unwrap() error { return ErrValidation }

func newValidationError(field, message string) error {
	return &ValidationError{Field: field, Message: message}
}
