# MIN-010 詳細設計メモ — Postgres リポジトリ基盤

作業中の検討メモ。決まったことはこのファイルに追記していく。
設計判断のうち重いものは、確定後に `docs/adr/` へ移す。

関連: [../backlog.md](../backlog.md) / [../05-architecture.md](../05-architecture.md) 2章 /
[../adr/0002-inventory-concurrency.md](../adr/0002-inventory-concurrency.md)

## 1. 目的

注文登録（MIN-020）が書けるように、**DBとのやりとりの窓口**を用意する。

窓口の形は「注文登録が何を必要とするか」から逆算して決まる。
そのため、まず注文登録の流れとデータを固めてからインターフェースを設計する。

## 2. 注文登録の流れ

APIが受け取るもの（[../04-api-spec.md](../04-api-spec.md) 3章）:

```json
{
  "channelCode": "EC_OWN",
  "locationCode": "WH-TOKYO",
  "customer": { "name": "山田 太郎", "email": "...", "phone": "..." },
  "shippingAddress": { "postalCode": "150-0001", ... },
  "lines": [
    { "skuCode": "MM-TOWEL-BL-M", "quantity": 2 },
    { "skuCode": "MM-MUG-WH-L",   "quantity": 1 }
  ]
}
```

処理の流れ:

| # | やること | 層 |
| --- | --- | --- |
| 1 | `skuCode` から SKU情報（ID・単価・status）を引く | postgres |
| 2 | `locationCode` `channelCode` から ID を引く | postgres |
| 3 | 在庫をロックして取る（`FOR UPDATE`） | postgres |
| 4 | 引当できるか判定する | **domain** |
| 5 | 在庫を更新する | postgres |
| 6 | 在庫変動履歴を書く | postgres |
| 7 | 注文・明細・遷移履歴を書く | postgres |

**1〜7 はひとつのトランザクション。** 4 で判定してから 5 で更新するまでの間に
他の注文が割り込むと売り越しになる（R-01）。

## 3. 必要なデータ

### 読むもの

| 対象 | キー | 取る項目 | なぜ要るか |
| --- | --- | --- | --- |
| SKU | `skuCode` | id, price_amount, status | 明細に注文時点の単価を記録する。停止中SKUは注文不可（FR-105） |
| 拠点 | `locationCode` | id | 在庫の特定に要る |
| チャネル | `channelCode` | id | `orders.channel_id` に要る |
| 在庫 | sku_id + location_id | id, on_hand, reserved, version | 引当判定に使う |

### 書くもの

| 対象 | 内容 | 要件 |
| --- | --- | --- |
| `inventory_items` | `reserved` を更新 | FR-202 |
| `inventory_movements` | 引当の履歴を1件（追記のみ） | FR-207 |
| `orders` | 注文本体 | FR-301 |
| `order_lines` | 明細。`sku_code` と `unit_price` は注文時点の値をコピー | FR-301 |
| `order_status_changes` | 「→ pending」の遷移を1件 | FR-309 |

## 4. 決めること

この4つが決まるとインターフェースの形が決まる。

### 論点1: SKU情報と在庫を、別々に取るか一度に取るか

| 案 | 内容 |
| --- | --- |
| A | SKU取得 → 在庫取得の2クエリ |
| B | `skus` と `inventory_items` を JOIN して1クエリ |

**判断材料:** 在庫のロックは `inventory_items` の行に対して取る。
JOIN したときにロック対象がどうなるか（`FOR UPDATE OF` の指定）を確認する必要がある。

**決定:** （未定）

**理由:**

### 論点2: `code` → `id` の変換を誰がやるか

| 案 | 内容 |
| --- | --- |
| A | usecase が先に `skuCode` → `skuID` を解決し、repository には id を渡す |
| B | repository が `skuCode` のまま受け取り、内部で解決する |

**判断材料:** 在庫不足のエラーには `skuCode` を含める必要がある
（[../04-api-spec.md](../04-api-spec.md) 3章の409レスポンス）。
`id` だけ持ち回ると、エラーを組み立てるときに `code` を引き直すことになる。

なお `domain.InventoryItem` は `SKUCode` フィールドを持っている。

**決定:** （未定）

**理由:**

### 論点3: 停止中SKUをどこで弾くか

| 案 | 内容 |
| --- | --- |
| A | SQL の `WHERE status = 'active'` で除外する |
| B | 取得はして、domain / usecase で判定する |

**判断材料:** A だと「SKUが存在しない」と「販売停止中」が区別できず、
どちらも同じエラーになる。オペレーターへの説明が変わるなら区別が要る。

`CLAUDE.md` の規約に「ビジネスルールの判断を postgres に書かない」とある。

**決定:** （未定）

**理由:**

### 論点4: 注文の保存を1メソッドにまとめるか、分けるか

| 案 | 内容 |
| --- | --- |
| A | `Save(ctx, order)` 1発。明細と遷移履歴も内部でまとめて INSERT |
| B | `SaveOrder` / `SaveLines` / `SaveStatusChange` に分ける |

**判断材料:** `domain.Order` は明細と履歴を内部に持つ集約。
集約は一体で保存されるのが自然だが、部分更新（状態遷移だけ）のときに
A だと過剰な書き込みになる可能性がある。

**決定:** （未定）

**理由:**

## 5. 決定後に書くもの

論点が固まってから着手する。

| 順 | ファイル | 内容 |
| --- | --- | --- |
| 1 | `usecase/ports.go` | インターフェース宣言。SQLは書かない |
| 2 | `postgres/errors.go` | PostgreSQLのエラー → ドメインエラーの変換 |
| 3 | `postgres/inventory_repository.go` | 在庫の取得・更新。`ORDER BY id FOR UPDATE` はここに1回だけ |
| 4 | `postgres/tx.go` | トランザクション境界の仕組み（別途 ADR-0004） |
| 5 | `postgres/order_repository.go` | 注文の保存 |
| 6 | `postgres/*_test.go` | 実DBに対するテスト |

## 6. 別途決める論点

トランザクションをどう引き回すか（A: 引数 / B: context / C: クロージャ）は
影響範囲が大きいため **ADR-0004** として単独で起票する。

現時点の議論の要点:

- A は `Tx` 抽象をきれいに作れず、型アサーションで実行時エラーに戻りやすい
- A の型検査は「何かの tx が渡された」ことしか保証せず、tx の取り違えは防げない
- C はクロージャに tx 紐付きのリポジトリを渡すため、取り違えが構造上起こせない
- C は commit/rollback を仕組みで保証でき、rollback 漏れによるロック滞留（A-07）を防げる

## 7. メモ・未解決

- `orderNumber`（`MM-20260601-000123`）の採番を誰がやるか未定。
  連番部分の生成にDBのシーケンスが要るなら repository の責務になる
- `actorID` の取得元が未定。認証基盤（MIN-014）が未着手のため、
  当面は固定値かリクエストパラメータで受ける必要がある
- testcontainers を CI で動かせるかは未検証。ここが最大の不確定要素
