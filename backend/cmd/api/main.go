// Command api は在庫・注文管理APIのエントリポイント。
// 依存の組み立てと起動・終了のみを担い、業務ロジックは持たない。
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/minatomart/inventory-api/internal/adapter/httpapi"
	"github.com/minatomart/inventory-api/internal/adapter/postgres"
	"github.com/minatomart/inventory-api/internal/platform/config"
)

func main() {
	if err := run(); err != nil {
		slog.Error("起動に失敗しました", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	setupLogger(cfg.LogLevel)

	// SIGTERM を受けたらこのコンテキストがキャンセルされる。
	// ECSはタスク停止時にSIGTERMを送るので、処理中のリクエストを捌ききってから終了する。
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := postgres.Connect(ctx, cfg.DatabaseURL, cfg.StatementTimeout)
	if err != nil {
		return err
	}
	defer db.Close()

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: httpapi.NewServer(db, cfg.Version).Routes(),

		// タイムアウトを明示しないと、遅いクライアントに接続を占有される。
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		slog.Info("APIサーバを起動しました",
			slog.String("port", cfg.Port),
			slog.String("version", cfg.Version),
		)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
		slog.Info("終了シグナルを受信しました。処理中のリクエストの完了を待ちます")
	}

	// グレースフルシャットダウン。処理中の注文登録が中断されると、
	// 在庫を引き当てたまま注文が返らない状態になりうるため、待ってから落とす。
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}

	slog.Info("正常に終了しました")
	return nil
}

// setupLogger は構造化JSONログを設定する。
// CloudWatch Logs Insights で検索するため、行指向のテキストではなくJSONにする。
func setupLogger(level string) {
	var lv slog.Level
	if err := lv.UnmarshalText([]byte(level)); err != nil {
		lv = slog.LevelInfo
	}

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: lv,
	})))
}
