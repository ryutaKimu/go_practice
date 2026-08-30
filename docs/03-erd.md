# ER設計

関連: [02-domain-model.md](02-domain-model.md) / マイグレーション: `backend/migrations/`

## 1. ER図

```mermaid
erDiagram
  categories ||--o{ products : "分類する"
  products   ||--|{ skus : "構成する"
  locations  ||--o{ inventory_items : "保管する"
  skus       ||--o{ inventory_items : "在庫を持つ"
  inventory_items ||--o{ inventory_movements : "変動履歴"
  channels   ||--o{ orders : "経由する"
  locations  ||--o{ orders : "出荷元"
  orders     ||--|{ order_lines : "明細"
  orders     ||--o{ order_status_changes : "遷移履歴"
  skus       ||--o{ order_lines : "参照される"
  actors     ||--o{ order_status_changes : "実行する"
  actors     ||--o{ inventory_movements : "実行する"

  categories {
    uuid id PK
    text name
    uuid parent_id FK "自己参照。NULLならルート"
  }
  products {
    uuid id PK
    text name
    text description
    uuid category_id FK
    text status "active | suspended"
    timestamptz created_at
    timestamptz updated_at
  }
  skus {
    uuid id PK
    uuid product_id FK
    text code UK "全体で一意"
    bigint price_amount "円。整数"
    jsonb attributes "色・サイズ等"
    text status "active | suspended"
  }
  locations {
    uuid id PK
    text code UK "WH-TOKYO 等"
    text name
    boolean is_active
  }
  inventory_items {
    uuid id PK
    uuid sku_id FK
    uuid location_id FK
    integer on_hand "CHECK >= 0"
    integer reserved "CHECK >= 0 かつ <= on_hand"
    integer version "楽観ロック"
  }
  inventory_movements {
    uuid id PK
    uuid inventory_item_id FK
    text movement_type "reserve|release|ship|receive|adjust"
    integer on_hand_delta
    integer reserved_delta
    text reason_code
    text note
    uuid order_id FK "引当系のみ。NULL可"
    uuid actor_id FK
    timestamptz occurred_at
  }
  channels {
    uuid id PK
    text code UK "EC_OWN | RAKUTEN | AMAZON"
    text name
    boolean is_active
  }
  orders {
    uuid id PK
    text order_number UK
    uuid channel_id FK
    uuid location_id FK
    text status
    text idempotency_key UK
    text customer_name "個人情報"
    text customer_email "個人情報"
    text customer_phone "個人情報"
    jsonb shipping_address "個人情報"
    bigint total_amount
    timestamptz placed_at
    timestamptz updated_at
  }
  order_lines {
    uuid id PK
    uuid order_id FK
    uuid sku_id FK
    text sku_code "注文時点のスナップショット"
    integer quantity "CHECK > 0"
    bigint unit_price "注文時点のスナップショット"
  }
  order_status_changes {
    uuid id PK
    uuid order_id FK
    text from_status "初回はNULL"
    text to_status
    uuid actor_id FK
    text reason
    timestamptz changed_at
  }
  actors {
    uuid id PK
    text type "staff | api_client"
    text email UK "staffのみ"
    text password_hash "staffのみ"
    text name
    text role "operator | admin"
    boolean is_active
  }
```

## 2. 設計上の決定と理由

### 2.1 金額を `bigint`（整数の円）で持つ

`numeric` や浮動小数点を使わない。日本円は最小単位が1円で小数を持たないため、
整数で保持すれば丸め誤差の考慮そのものが不要になる。
将来多通貨対応する場合は `currency` カラムと最小単位の指数を追加する。

### 2.2 在庫の不変条件をDB制約に落とす

```sql
CHECK (on_hand >= 0)
CHECK (reserved >= 0)
CHECK (reserved <= on_hand)
```

アプリ側でも検証するが、**DB側にも置く**。
バッチ処理や手動SQLなどアプリを経由しない経路があっても、売り越し状態のデータが物理的に作れなくなる。
R-01（売り越しゼロ）に対する最後の防波堤。

### 2.3 `inventory_items` に一意制約

```sql
UNIQUE (sku_id, location_id)
```

SKU×拠点の在庫レコードが重複すると、引当可能数の計算が二重になって破綻する。

### 2.4 履歴テーブルは追記のみ

`inventory_movements` と `order_status_changes` は INSERT のみ。
UPDATE/DELETE をアプリから発行しない。R-02（全変更の追跡）を満たすため。
運用上も、これらのテーブルには専用DBロールから UPDATE/DELETE 権限を与えない。

### 2.5 注文明細に `sku_code` と `unit_price` をコピー

正規化の原則からは冗長だが、意図的にコピーする。
商品マスタの価格が改定されても、過去の注文金額が変わってはならない。
`sku_id` の外部キーは残すので、現在の商品情報への参照もたどれる。

### 2.6 冪等キーを一意制約で守る

```sql
UNIQUE (idempotency_key)
```

FR-302。アプリ側で「既存チェック→なければ作成」とすると競合時に二重作成が起きる。
一意制約違反を捕まえて既存の注文を返す実装にする（詳細は [ADR-0003](adr/0003-idempotency.md)）。

## 3. 主なインデックス

| テーブル | インデックス | 理由 |
| --- | --- | --- |
| `skus` | `UNIQUE (code)` | SKUコード検索 (FR-103) |
| `products` | `(category_id, status)` | カテゴリ絞り込み |
| `inventory_items` | `UNIQUE (sku_id, location_id)` | 引当時の一意特定 |
| `inventory_movements` | `(inventory_item_id, occurred_at DESC)` | 履歴照会 (FR-207) |
| `orders` | `UNIQUE (order_number)` | 注文番号検索 (FR-303) |
| `orders` | `UNIQUE (idempotency_key)` | 冪等性 (FR-302) |
| `orders` | `(status, placed_at DESC)` | ステータス別一覧 |
| `orders` | `(channel_id, placed_at DESC)` | チャネル別集計 |
| `order_lines` | `(order_id)` | 明細取得 |
| `order_status_changes` | `(order_id, changed_at)` | 遷移履歴 (FR-309) |

`orders.customer_name` の部分一致検索（FR-303）は、件数が増えたら `pg_trgm` の GIN インデックスを検討する。
フェーズ1では前方一致で実装し、負荷試験の結果を見て判断する。

## 4. データ量の見積もり

| テーブル | 初期 | 年間増加 | 3年後 |
| --- | --- | --- | --- |
| `products` | 3,000 | +600 | 4,800 |
| `skus` | 12,000 | +2,400 | 19,200 |
| `inventory_items` | 24,000 | +4,800 | 38,400 |
| `orders` | 1,460,000（過去2年移行） | +900,000 | 4,160,000 |
| `order_lines` | 3,800,000 | +2,300,000 | 10,700,000 |
| `inventory_movements` | 0 | +5,000,000 | 15,000,000 |

`inventory_movements` が最も増える。3年で1,500万行。
`occurred_at` によるレンジパーティションを将来の選択肢として想定しておく（フェーズ1では単一テーブル）。
