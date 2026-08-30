package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type ctxKey string

const requestIDKey ctxKey = "requestID"

// RequestID は全リクエストに一意なIDを振り、コンテキストとレスポンスヘッダの両方に載せる。
// 障害調査時に、顧客から提示されたIDでログを一発で引けるようにするため
// （docs/05-architecture.md 6章）。
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// クライアントが付けてきたIDがあれば引き継ぐ。ECサイト側のログと突き合わせられる。
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = uuid.NewString()
		}

		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
	})
}

func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// statusRecorder は書き込まれたステータスコードを覚えておくための ResponseWriter。
// http.ResponseWriter は書き込んだステータスを読み出せないため必要になる。
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	// ハンドラが WriteHeader を呼ばずに Write した場合、Goは暗黙に200を送る。
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

// AccessLog は構造化JSONでアクセスログを出す。
// 個人情報を含みうるリクエストボディやクエリ値は出力しない(NFR-06)。
func AccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r)

		slog.InfoContext(r.Context(), "http request",
			slog.String("request_id", RequestIDFromContext(r.Context())),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", rec.status),
			slog.Int("bytes", rec.bytes),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
		)
	})
}

// Recover はハンドラのpanicを500に変換する。
// 1リクエストのバグでプロセス全体が落ちると、無関係な注文まで失敗するため。
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.ErrorContext(r.Context(), "panic recovered",
					slog.Any("panic", rec),
					slog.String("request_id", RequestIDFromContext(r.Context())),
					slog.String("path", r.URL.Path),
				)
				writeJSON(w, http.StatusInternalServerError, errorBody{errorDetail{
					Code: "INTERNAL_ERROR", Message: "サーバー内部エラーが発生しました",
				}})
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// Chain はミドルウェアを適用する。先に渡したものが外側（先に実行される）になる。
func Chain(h http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}
