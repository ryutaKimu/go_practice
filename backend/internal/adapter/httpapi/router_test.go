package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type stubDB struct{ err error }

func (s stubDB) Ping(context.Context) error { return s.err }

func TestHealthz_DBが落ちていても200を返す(t *testing.T) {
	// liveness で依存先を見ない理由: DB障害でタスクを再起動しても復旧しないため。
	// MIN-011 usecase実装後に差し替え
	srv := NewServer(stubDB{err: errors.New("connection refused")}, "test", nil, nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestReadyz(t *testing.T) {
	tests := []struct {
		name       string
		dbErr      error
		wantStatus int
	}{
		{"DB正常", nil, http.StatusOK},
		{"DB異常", errors.New("connection refused"), http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// MIN-011 usecase実装後に差し替え
			srv := NewServer(stubDB{err: tt.dbErr}, "test", nil, nil)
			rec := httptest.NewRecorder()

			srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestRequestID(t *testing.T) {
	// MIN-011 usecase実装後に差し替え
	srv := NewServer(stubDB{}, "test", nil, nil)

	t.Run("未指定なら生成される", func(t *testing.T) {
		rec := httptest.NewRecorder()

		srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

		if rec.Header().Get("X-Request-ID") == "" {
			t.Error("X-Request-ID ヘッダが空です")
		}
	})

	t.Run("クライアント指定のIDを引き継ぐ", func(t *testing.T) {
		// ECサイト側のログと突き合わせられるようにするため。
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		req.Header.Set("X-Request-ID", "ec-site-12345")

		srv.Routes().ServeHTTP(rec, req)

		if got := rec.Header().Get("X-Request-ID"); got != "ec-site-12345" {
			t.Errorf("X-Request-ID = %q, want ec-site-12345", got)
		}
	})
}

func TestRecover_panicを500に変換する(t *testing.T) {
	panicking := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("想定外の不具合")
	})
	handler := Chain(panicking, RequestID, Recover)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/orders", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	if got := rec.Body.String(); !strings.Contains(got, "INTERNAL_ERROR") {
		t.Errorf("body = %s, INTERNAL_ERROR を期待", got)
	}
}
