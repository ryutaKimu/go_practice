# ミナトマート 在庫・注文管理API — 開発規約

このファイルは Claude Code が毎回読み込む。作業前にここと該当ドキュメントを確認すること。

## この案件について

生活雑貨EC「ミナトマート」の在庫・注文の中核APIをGoで新規構築し、
社内オペレーター向け管理画面をReactで作る案件。既存PHPモノリスから段階的に切り出す。

**この案件の成否を決める要件は R-01「在庫の売り越しをゼロにする」。**
迷ったときはこれを守る方に倒す。詳細は [docs/00-rfp.md](docs/00-rfp.md)。

## ドキュメントの読み分け

| 知りたいこと | 見る場所 |
| --- | --- |
| 案件の背景・クライアントの課題 | [docs/00-rfp.md](docs/00-rfp.md) |
| 何を作るか（機能一覧・非機能要件） | [docs/01-requirements.md](docs/01-requirements.md) |
| ドメインのルール（在庫・注文の振る舞い） | [docs/02-domain-model.md](docs/02-domain-model.md) |
| テーブル設計 | [docs/03-erd.md](docs/03-erd.md) |
| APIの形 | [docs/04-api-spec.md](docs/04-api-spec.md) / [api/openapi.yaml](api/openapi.yaml) |
| なぜその構成なのか | [docs/05-architecture.md](docs/05-architecture.md) |
| **なぜその判断をしたのか** | [docs/adr/](docs/adr/) |
| 運用・障害対応・移行 | [docs/06-operations.md](docs/06-operations.md) |
| 今やるべきこと | [docs/backlog.md](docs/backlog.md) |

## 開発環境

```bash
# 全部起動（API + Postgres + 管理画面）
docker compose -f deploy/docker-compose.yml up

# バックエンドのテスト
cd backend && go test -race ./...

# フロントエンド
cd frontend && npm run dev
```

| URL | 内容 |
| --- | --- |
| http://localhost:8080/healthz | API 死活確認 |
| http://localhost:5173 | 管理画面 |
| postgres://minato:minato_local_only@localhost:55432/minato | ローカルDB |

開発用ログイン: `admin@minatomart.example` / `operator@minatomart.example`（パスワードはいずれも `password123`）

`migrate` はマイグレーション適用後に終了する使い捨てコンテナなので、`Exited (0)` が正常な状態。
`docker compose ps` には出てこない（`-a` を付けると見える）。詳細は [README.md](README.md)。

## チケットの進め方

1. [docs/backlog.md](docs/backlog.md) から着手するチケットを選び、状態を `進行中` にする
2. ブランチを切る: `feat/MIN-020-place-order`（`<type>/<ID>-<英語の要約>`）
3. **テストを先に書く。** 受入条件をテスト名にそのまま落とす
4. 実装する
5. `go test -race ./...` と `npm run typecheck && npm run lint` を通す
6. PRを作る。本文には「どのチケットか」「なぜこの実装にしたか」「何をテストしたか」を書く
7. `/code-review` でレビューする
8. マージ後、backlog の状態を `完了` にする

チケット1件は1〜3日で終わる大きさに保つ。それより大きくなりそうなら分割する。

## コーディング規約

### 全体

- **コメントは「なぜ」を書く。** 「何をしているか」はコードを読めばわかる
- 要件やADRに紐づく判断には、該当ドキュメントへの参照を書く（例: `// ADR-0002 により悲観ロック`）
- 日本語のコメント・テスト名でよい。クライアントに引き継ぐため

### Go

- レイヤの依存方向を守る: `httpapi` → `usecase` → `domain`、`postgres` → `usecase(ports)` → `domain`
- **`domain` パッケージは他の内部パッケージをimportしない。** DB・HTTP・時刻取得も持ち込まない
- **ビジネスルールの判断を `httpapi` や `postgres` に書かない。** 「在庫が足りなければエラー」は `domain` の責務
- 時刻は引数で受け取る（`now time.Time`）。`time.Now()` をドメイン層で呼ばない。テストが書けなくなる
- エラーは `fmt.Errorf("...: %w", err)` でラップして文脈を足す。握りつぶさない
- HTTPステータスの決定は `httpapi/errors.go` の `mapError` 1箇所だけ

### 用語

[docs/02-domain-model.md](docs/02-domain-model.md) 6章のユビキタス言語に従う。特に:

- 引当 = `Reserve`（`allocate` は使わない。注文ステータスの `allocated` と紛らわしいため）
- 実在庫数 = `OnHand` / 引当済数 = `Reserved` / 引当可能数 = `Available`

### SQL・マイグレーション

- SQLは `internal/adapter/postgres` 配下にのみ書く
- マイグレーションは up と down の両方を書き、**down がローカルで通ることを確認する**
- 後方互換を保つ（Expand and Contract）。カラム削除とリネームを1デプロイでやらない
- 詳細は [docs/06-operations.md](docs/06-operations.md) 4章

### TypeScript / React

- `strict` を緩めない。`any` を使わない
- サーバー状態は TanStack Query に置く。グローバル状態管理ライブラリは足さない
- API呼び出しは `src/api/` に集約する。コンポーネントから直接 `fetch` しない

## テスト方針

| 対象 | 書き方 |
| --- | --- |
| `domain` | 純粋なユニットテスト。全分岐を網羅する |
| `usecase` | リポジトリをモックして手続きを検証 |
| `postgres` | testcontainers で実DBに対して実行 |
| 在庫の並行制御 | 並列実行テスト必須（[ADR-0002](docs/adr/0002-inventory-concurrency.md)） |

- テスト名は日本語で、何を保証しているかを書く（`Test在庫不足の注文は拒否される`）
- **在庫を操作するテストでは、必ず不変条件（`Reserved <= OnHand`）を最後に確認する**
- カバレッジは70%以上（NFR-08）。CIで落とす

## やってはいけないこと

- `domain` の不変条件チェックを迂回して直接フィールドを書き換える
- `Order.Status` に直接代入する（`transitionTo` を通す。履歴が残らなくなる）
- 引当だけを行うAPIを公開する（注文と紐づかない在庫が生まれる。[docs/04-api-spec.md](docs/04-api-spec.md) 2章）
- 個人情報（顧客名・住所・電話番号・メール）をログに出力する（NFR-06）
- CORS の許可オリジンにワイルドカード（`*`）を使う（認証トークンと個人情報を扱うため）
- 新しいリクエストヘッダを追加したのに `httpapi/cors.go` の `allowedHeaders` に足さない
  （プリフライトで弾かれ、ブラウザには `Failed to fetch` としか出ない）
- 500エラーのレスポンスに内部エラーの詳細を含める
- 冪等キーが無いときにサーバー側で生成する（[ADR-0003](docs/adr/0003-idempotency.md)）
- セール期間中にデプロイする（開始48時間前から凍結）
