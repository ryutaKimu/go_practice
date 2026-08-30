# ミナトマート 在庫・注文管理API

生活雑貨EC「ミナトマート」の在庫・注文を扱う中核APIと、社内オペレーター向け管理画面。
既存PHPモノリスから段階的に切り出す想定の**擬似案件**。

> この案件は実開発の進め方を練習するための仮想案件です。クライアント・数値・要件はすべて架空です。

## この案件が解こうとしている問題

現行システムは在庫テーブルを自社ECとモールで別々に持ち、同期が15分間隔のバッチであるため、
セール時に**売り越しが月20〜50件**発生している。これをゼロにすることが最優先要件（R-01）。

## 構成

| ディレクトリ | 内容 |
| --- | --- |
| `docs/` | 要件定義・設計・運用ドキュメント |
| `docs/adr/` | アーキテクチャ上の判断記録 |
| `backend/` | Go 1.26 / net/http / pgx |
| `frontend/` | React 19 / TypeScript / Vite |
| `api/` | OpenAPI 定義 |
| `deploy/` | Docker Compose（ローカル環境） |

## 起動

```bash
docker compose -f deploy/docker-compose.yml up
```

| URL | 内容 |
| --- | --- |
| http://localhost:8080/healthz | API 死活確認 |
| http://localhost:5173 | 管理画面 |

### `migrate` コンテナが一覧に出てこない場合

**正常です。** `migrate` はマイグレーションを適用したら終了する使い捨てコンテナで、常駐しません。
`docker compose ps` は終了済みコンテナを隠すため、`-a` を付けて確認します。

```bash
docker compose -f deploy/docker-compose.yml ps -a
# minato-inventory-migrate-1 ... Exited (0)   ← これが正常な状態

docker compose -f deploy/docker-compose.yml logs migrate
# 1/u init_schema / 2/u seed_master_data、2回目以降は "no change"
```

`api` は migrate の**正常終了を待ってから**起動する設定なので、
api が healthy であれば migrate は成功しています。逆に migrate が失敗すれば api は起動しません。

スキーマが入ったかを直接確かめるには:

```bash
docker compose -f deploy/docker-compose.yml exec db psql -U minato -d minato -c "\dt"
```

### 管理画面に「APIに接続できません: Failed to fetch」と出る場合

APIが落ちているとは限りません。**CORSの可能性が高い**です。
`curl` では成功するのにブラウザだけ失敗する場合はこれです。

```bash
# ブラウザと同じ条件で確認する。Access-Control-Allow-Origin が返るのが正常。
curl -i -H "Origin: http://localhost:5173" http://localhost:8080/healthz | grep -i access-control
```

ヘッダが返らない場合、APIの `ALLOWED_ORIGINS` に管理画面のオリジンが入っていません。
`deploy/docker-compose.yml` の `api` サービスの環境変数を確認してください。
APIの起動ログにも許可オリジンが出ます。

```bash
docker compose -f deploy/docker-compose.yml logs api | grep allowed_origins
```

管理画面のポートを既定（5173）から変えた場合は、`ALLOWED_ORIGINS` も合わせて変更が必要です。

### DBを初期状態に戻す

```bash
docker compose -f deploy/docker-compose.yml down -v   # -v でボリュームごと削除
docker compose -f deploy/docker-compose.yml up
```

### Docker を使わずバックエンドだけ動かす場合

```bash
cd backend
export DATABASE_URL="postgres://minato:minato_local_only@localhost:55432/minato?sslmode=disable"
go run ./cmd/api
```

## テスト

```bash
cd backend  && go test -race ./...
cd frontend && npm run typecheck && npm run lint
```

## ドキュメントの読み順

初めて読むなら上から順に。

1. [docs/00-rfp.md](docs/00-rfp.md) — クライアントの課題と依頼内容
2. [docs/01-requirements.md](docs/01-requirements.md) — 機能要件・非機能要件
3. [docs/02-domain-model.md](docs/02-domain-model.md) — 在庫と注文のルール
4. [docs/03-erd.md](docs/03-erd.md) — テーブル設計
5. [docs/04-api-spec.md](docs/04-api-spec.md) — APIの形
6. [docs/05-architecture.md](docs/05-architecture.md) — システム構成とレイヤ
7. [docs/06-operations.md](docs/06-operations.md) — 移行・監視・障害対応
8. [docs/adr/](docs/adr/) — なぜその判断をしたか

開発規約は [CLAUDE.md](CLAUDE.md)。次にやることは [docs/backlog.md](docs/backlog.md)。

## 進捗

| Sprint | 内容 | 状態 |
| --- | --- | --- |
| Sprint 0 | 基盤・ドメイン層・HTTP骨格 | 完了 |
| Sprint 1 | 商品と在庫の読み取り、認証 | 未着手 |
| Sprint 2 | 注文登録と在庫引当（**中核**） | 未着手 |
| Sprint 3 | 注文のライフサイクル | 未着手 |
| Sprint 4 | 運用機能と非機能 | 未着手 |
| Sprint 5 | 移行と引き継ぎ | 未着手 |
