package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/minatomart/inventory-api/internal/domain"
)

// errorBody は全エラーレスポンスの共通形式（docs/05-architecture.md 5章）。
type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details []any  `json:"details,omitempty"`
}

type shortageDetail struct {
	SKUCode   string `json:"skuCode"`
	Requested int    `json:"requested"`
	Available int    `json:"available"`
}

type fieldDetail struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// writeError はドメインエラーをHTTPレスポンスへ変換する唯一の場所。
// ハンドラ側でステータスコードを個別に決めさせない。判断が散ると、
// 同じ種類のエラーが呼び出し箇所によって違うコードで返るようになるため。
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	status, body := mapError(err)

	if status >= 500 {
		// 内部エラーの詳細はログにだけ出す。レスポンスには含めない。
		// SQLエラーの原文などが外部に漏れるのを防ぐ。
		slog.ErrorContext(r.Context(), "unhandled error",
			slog.String("error", err.Error()),
			slog.String("path", r.URL.Path),
		)
	}

	writeJSON(w, status, body)
}

func mapError(err error) (int, errorBody) {
	var stockErr *domain.InsufficientStockError
	if errors.As(err, &stockErr) {
		details := make([]any, 0, len(stockErr.Shortages))
		for _, s := range stockErr.Shortages {
			details = append(details, shortageDetail{
				SKUCode: s.SKUCode, Requested: s.Requested, Available: s.Available,
			})
		}
		return http.StatusConflict, errorBody{errorDetail{
			Code: "INSUFFICIENT_STOCK", Message: "在庫が不足しています", Details: details,
		}}
	}

	var validationErr *domain.ValidationError
	if errors.As(err, &validationErr) {
		return http.StatusBadRequest, errorBody{errorDetail{
			Code:    "VALIDATION_ERROR",
			Message: "入力内容に誤りがあります",
			Details: []any{fieldDetail{Field: validationErr.Field, Message: validationErr.Message}},
		}}
	}

	var transitionErr *domain.StatusTransitionError
	if errors.As(err, &transitionErr) {
		return http.StatusConflict, errorBody{errorDetail{
			Code:    "INVALID_STATUS_TRANSITION",
			Message: "現在のステータスからは実行できない操作です",
			Details: []any{map[string]string{
				"from": string(transitionErr.From),
				"to":   string(transitionErr.To),
			}},
		}}
	}

	switch {
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound, errorBody{errorDetail{
			Code: "NOT_FOUND", Message: "対象が見つかりません",
		}}
	case errors.Is(err, domain.ErrForbidden):
		return http.StatusForbidden, errorBody{errorDetail{
			Code: "FORBIDDEN", Message: "この操作を行う権限がありません",
		}}
	case errors.Is(err, domain.ErrValidation):
		return http.StatusBadRequest, errorBody{errorDetail{
			Code: "VALIDATION_ERROR", Message: "入力内容に誤りがあります",
		}}
	case errors.Is(err, domain.ErrInsufficientStock):
		return http.StatusConflict, errorBody{errorDetail{
			Code: "INSUFFICIENT_STOCK", Message: "在庫が不足しています",
		}}
	case errors.Is(err, domain.ErrInvalidStatusTransition):
		return http.StatusConflict, errorBody{errorDetail{
			Code: "INVALID_STATUS_TRANSITION", Message: "現在のステータスからは実行できない操作です",
		}}
	default:
		return http.StatusInternalServerError, errorBody{errorDetail{
			Code: "INTERNAL_ERROR", Message: "サーバー内部エラーが発生しました",
		}}
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if body == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		// ヘッダは送信済みなのでステータスは変えられない。ログに残すだけ。
		slog.Error("failed to encode response", slog.String("error", err.Error()))
	}
}
