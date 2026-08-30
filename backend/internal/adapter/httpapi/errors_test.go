package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/minatomart/inventory-api/internal/domain"
)

// ドメインエラーとHTTPステータスの対応は docs/05-architecture.md 5章の表が正。
// 実装がその表からずれていないことを固定する。
func TestMapError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"在庫不足", domain.ErrInsufficientStock, http.StatusConflict, "INSUFFICIENT_STOCK"},
		{"不正な状態遷移", domain.ErrInvalidStatusTransition, http.StatusConflict, "INVALID_STATUS_TRANSITION"},
		{"未検出", domain.ErrNotFound, http.StatusNotFound, "NOT_FOUND"},
		{"入力不備", domain.ErrValidation, http.StatusBadRequest, "VALIDATION_ERROR"},
		{"権限なし", domain.ErrForbidden, http.StatusForbidden, "FORBIDDEN"},
		{"想定外", errors.New("boom"), http.StatusInternalServerError, "INTERNAL_ERROR"},
		// ラップされていても同じ扱いになること。ユースケース層が文脈を足しても壊れない。
		{"ラップされた在庫不足", fmt.Errorf("place order: %w", domain.ErrInsufficientStock),
			http.StatusConflict, "INSUFFICIENT_STOCK"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := mapError(tt.err)

			if status != tt.wantStatus {
				t.Errorf("status = %d, want %d", status, tt.wantStatus)
			}
			if body.Error.Code != tt.wantCode {
				t.Errorf("code = %q, want %q", body.Error.Code, tt.wantCode)
			}
		})
	}
}

func TestMapError_在庫不足は不足SKUを全件返す(t *testing.T) {
	err := &domain.InsufficientStockError{Shortages: []domain.StockShortage{
		{SKUCode: "MM-TOWEL-BL-M", Requested: 2, Available: 1},
		{SKUCode: "MM-MUG-WH-L", Requested: 5, Available: 0},
	}}

	status, body := mapError(err)

	if status != http.StatusConflict {
		t.Errorf("status = %d, want 409", status)
	}
	// 最初の1件で打ち切らない。オペレーターが一度で代替提案できるようにするため。
	if len(body.Error.Details) != 2 {
		t.Fatalf("details の件数 = %d, want 2", len(body.Error.Details))
	}
	first, ok := body.Error.Details[0].(shortageDetail)
	if !ok {
		t.Fatalf("details[0] の型 = %T", body.Error.Details[0])
	}
	if first.SKUCode != "MM-TOWEL-BL-M" || first.Requested != 2 || first.Available != 1 {
		t.Errorf("details[0] = %+v", first)
	}
}

func TestMapError_入力不備はフィールド名を返す(t *testing.T) {
	err := &domain.ValidationError{Field: "reason", Message: "キャンセル理由は必須です"}

	_, body := mapError(err)

	detail, ok := body.Error.Details[0].(fieldDetail)
	if !ok {
		t.Fatalf("details[0] の型 = %T", body.Error.Details[0])
	}
	if detail.Field != "reason" {
		t.Errorf("field = %q, want reason", detail.Field)
	}
}

// 500系のレスポンスに内部情報が漏れていないことを確認する。
// SQLエラーの原文などが外部に出ると、テーブル構成の推測材料になる。
func TestWriteError_内部エラーの詳細を漏らさない(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", nil)
	internal := errors.New(`pq: relation "inventory_items" does not exist`)

	writeError(rec, req, internal)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "inventory_items") || strings.Contains(body, "pq:") {
		t.Errorf("レスポンスに内部情報が含まれている: %s", body)
	}

	var parsed errorBody
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatalf("レスポンスがJSONとして不正: %v", err)
	}
	if parsed.Error.Code != "INTERNAL_ERROR" {
		t.Errorf("code = %q, want INTERNAL_ERROR", parsed.Error.Code)
	}
}
