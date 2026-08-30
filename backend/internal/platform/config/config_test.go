package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	t.Run("必須の値があれば既定値で埋まる", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://localhost/test")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load err = %v", err)
		}

		if cfg.Port != "8080" {
			t.Errorf("Port = %q, want 8080", cfg.Port)
		}
		if cfg.LogLevel != "info" {
			t.Errorf("LogLevel = %q, want info", cfg.LogLevel)
		}
		// ADR-0002 の「statement_timeout を3秒」に対応する既定値。
		if cfg.StatementTimeout != 3*time.Second {
			t.Errorf("StatementTimeout = %v, want 3s", cfg.StatementTimeout)
		}
		if cfg.ShutdownTimeout != 15*time.Second {
			t.Errorf("ShutdownTimeout = %v, want 15s", cfg.ShutdownTimeout)
		}
	})

	t.Run("環境変数で上書きできる", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://localhost/test")
		t.Setenv("PORT", "9000")
		t.Setenv("LOG_LEVEL", "debug")
		t.Setenv("APP_VERSION", "1.4.2")
		t.Setenv("STATEMENT_TIMEOUT_SECONDS", "5")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load err = %v", err)
		}

		if cfg.Port != "9000" {
			t.Errorf("Port = %q, want 9000", cfg.Port)
		}
		if cfg.Version != "1.4.2" {
			t.Errorf("Version = %q, want 1.4.2", cfg.Version)
		}
		if cfg.StatementTimeout != 5*time.Second {
			t.Errorf("StatementTimeout = %v, want 5s", cfg.StatementTimeout)
		}
	})

	// 設定漏れのまま起動して、最初のリクエストで初めて気づくのを避ける。
	t.Run("DATABASE_URLがなければ起動を止める", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "")

		_, err := Load()

		if err == nil {
			t.Fatal("エラーを期待したが nil だった")
		}
		if !strings.Contains(err.Error(), "DATABASE_URL") {
			t.Errorf("エラーメッセージに DATABASE_URL が含まれていない: %v", err)
		}
	})

	t.Run("不正なタイムアウト値を拒否する", func(t *testing.T) {
		tests := []struct {
			name  string
			value string
		}{
			{"数値でない", "abc"},
			{"ゼロ", "0"},
			{"負の値", "-1"},
			{"単位付き", "3s"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Setenv("DATABASE_URL", "postgres://localhost/test")
				t.Setenv("STATEMENT_TIMEOUT_SECONDS", tt.value)

				if _, err := Load(); err == nil {
					t.Fatalf("STATEMENT_TIMEOUT_SECONDS=%q でエラーを期待したが nil だった", tt.value)
				}
			})
		}
	})
}
