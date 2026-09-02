package httpapi

import (
	"context"
	"net/http"

	"github.com/minatomart/inventory-api/internal/usecase"
)

// HealthChecker は /readyz が確認する依存先。今はDBのみ。
type HealthChecker interface {
	Ping(ctx context.Context) error
}

type Server struct {
	db       HealthChecker
	version  string
	cors     *CORS
	products usecase.ProductUsecase
}

func NewServer(db HealthChecker, version string, allowedOrigins []string, products usecase.ProductUsecase) *Server {
	return &Server{
		db:       db,
		version:  version,
		cors:     NewCORS(allowedOrigins),
		products: products,
	}
}

// Routes はルーティングを組み立てる。Go 1.22+ のパターンルーティングを使い、
// 外部のルータライブラリを入れていない（docs/05-architecture.md 3章）。
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	// 監視系は認証不要。ALBのヘルスチェックが叩くため。
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /readyz", s.handleReadyz)

	// TODO(MIN-011以降): /api/v1 配下の業務エンドポイントを追加する。
	// 一覧は docs/04-api-spec.md 2章。
	mux.HandleFunc("GET /api/v1/products", s.handleListProducts)

	// CORS は RequestID より内側に置く。プリフライトにも request_id を振って
	// ログに残すため（プリフライトが弾かれた場合の調査に要る）。
	return Chain(mux, RequestID, AccessLog, Recover, s.cors.Middleware)
}

// handleHealthz はプロセスが生きているかだけを返す。依存先は見ない。
// DBが落ちている間にタスクを再起動させても復旧しないため、
// liveness では依存先を確認しない。
func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"version": s.version,
	})
}

// handleReadyz はリクエストを受けられる状態かを返す。DB接続を確認する。
// 失敗中はALBがこのタスクへの振り分けを止める。
func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if err := s.db.Ping(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "unavailable",
			"reason": "database unreachable",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
