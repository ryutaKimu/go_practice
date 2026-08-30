package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DB は接続プールの薄いラッパー。リポジトリ実装はこれを受け取る。
type DB struct {
	pool *pgxpool.Pool
}

// Connect は接続プールを作る。statementTimeout は1クエリの上限で、
// 在庫引当の悲観ロックが想定外に長引いた場合の安全弁になる（ADR-0002）。
func Connect(ctx context.Context, dsn string, statementTimeout time.Duration) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("DSNの解析に失敗: %w", err)
	}

	// 接続ごとに statement_timeout を設定する。DSN側に書く方法もあるが、
	// 環境変数で調整できるようにここで指定する。
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = fmt.Sprintf("%d", statementTimeout.Milliseconds())

	// ECSタスク1つあたりの上限。Aurora の max_connections を
	// タスク数（最大8）で割った値を超えないようにする。
	cfg.MaxConns = 20
	cfg.MinConns = 2
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("接続プールの作成に失敗: %w", err)
	}

	// 起動時に疎通を確認する。DBに繋がらないまま起動を成功させると、
	// ALBが正常なタスクとして扱ってしまう。
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("データベースへの疎通確認に失敗: %w", err)
	}

	return &DB{pool: pool}, nil
}

// Ping は /readyz から呼ばれる疎通確認。
func (db *DB) Ping(ctx context.Context) error {
	return db.pool.Ping(ctx)
}

func (db *DB) Close() {
	db.pool.Close()
}

// Pool はリポジトリ実装がクエリを発行するために使う。
func (db *DB) Pool() *pgxpool.Pool {
	return db.pool
}
