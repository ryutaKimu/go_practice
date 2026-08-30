package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config はアプリ起動に必要な設定。すべて環境変数から読む。
// 設定ファイルを使わないのは、ECSのタスク定義で完結させるため。
type Config struct {
	Port            string
	DatabaseURL     string
	LogLevel        string
	Version         string
	ShutdownTimeout time.Duration

	// StatementTimeout は1クエリの上限時間。
	// 在庫引当は悲観ロックを取るため、想定外の長時間ロックを断ち切る安全弁になる
	// （ADR-0002 実装上の必須事項3）。
	StatementTimeout time.Duration
}

// Load は環境変数を読み込む。必須の値が欠けていればエラーを返し、起動させない。
// 設定漏れのまま起動して、最初のリクエストで初めて気づくのを避けるため。
func Load() (Config, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL は必須です")
	}

	shutdownTimeout, err := durationEnv("SHUTDOWN_TIMEOUT_SECONDS", 15*time.Second)
	if err != nil {
		return Config{}, err
	}
	statementTimeout, err := durationEnv("STATEMENT_TIMEOUT_SECONDS", 3*time.Second)
	if err != nil {
		return Config{}, err
	}

	return Config{
		Port:             envOr("PORT", "8080"),
		DatabaseURL:      dbURL,
		LogLevel:         envOr("LOG_LEVEL", "info"),
		Version:          envOr("APP_VERSION", "dev"),
		ShutdownTimeout:  shutdownTimeout,
		StatementTimeout: statementTimeout,
	}, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func durationEnv(key string, fallback time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}

	seconds, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s は整数（秒）である必要があります: %q", key, v)
	}
	if seconds <= 0 {
		return 0, fmt.Errorf("%s は1以上である必要があります: %d", key, seconds)
	}

	return time.Duration(seconds) * time.Second, nil
}
