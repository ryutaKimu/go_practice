package httpapi

import (
	"context"
	"net/http"
)

// HealthChecker は /readyz が確認する依存先。今はDBのみ。
type HealthChecker interface {
	Ping(ctx context.Context) error
}

type Server struct {
	db      HealthChecker
	version string
}

func NewServer(db HealthChecker, version string) *Server {
	return &Server{db: db, version: version}
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

	return Chain(mux, RequestID, AccessLog, Recover)
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
