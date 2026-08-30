# ドメインモデル

関連: [01-requirements.md](01-requirements.md) / [03-erd.md](03-erd.md) / [adr/0002-inventory-concurrency.md](adr/0002-inventory-concurrency.md)

## 1. 集約の切り方

| 集約 | 集約ルート | 不変条件 |
| --- | --- | --- |
| Product | Product | SKUを1件以上持つ。SKUコードは全体で一意 |
| Inventory | InventoryItem (SKU×拠点) | `onHand >= reserved >= 0` を常に満たす |
| Order | Order | 明細を1件以上持つ。状態遷移は定義された経路のみ |

### 1.1 在庫と注文を別々の集約にする理由

ライフサイクルが違うため。在庫は注文がなくても存在し（入荷・棚卸で動く）、
ひとつの在庫は多数の注文から参照される。
注文が在庫を内包する形にすると、同じ在庫が複数の注文の中に重複して存在することになる。

### 1.2 売り越し防止のルールを在庫側に置く理由

**ここがこのシステムの設計の核**で、R-01（売り越しゼロ）が達成できるかどうかを決めている。

「この注文を引き当ててよいか」の判定を、注文側と在庫側のどちらに置くかという選択がある。

**注文側に置いた場合** — 判定のたびに、同じSKUを引き当てている他の注文を全部数える必要がある。

```sql
SELECT SUM(quantity) FROM order_lines l
  JOIN orders o ON o.id = l.order_id
 WHERE l.sku_id = ? AND o.status IN ('pending', 'allocated');
```

対象は数百〜数千行になる。これを全部ロックするのは現実的でないうえ、
判定の最中に新しい注文が入れば数え直しになる。

**在庫側に置いた場合** — `InventoryItem` が `reserved`（引当済数）を持ち、
`reserved <= onHand` を不変条件とする。判定は1行を読んで比較するだけで済む。

```
available() = onHand - reserved  →  qty と比較する
```

`reserved` は本来、注文から計算できる導出値である。**それをあえて在庫側に実体化して持つ。**
冗長だが意図的であり、これによって

1. 売り越し判定が InventoryItem 1件の中で完結する（他の注文を見なくてよい）
2. ロック対象がその1行だけで済む
3. `SELECT ... FOR UPDATE` による悲観ロックが成立する（[ADR-0002](adr/0002-inventory-concurrency.md)）

**「整合性を在庫側に寄せる」とは、判定に必要な情報を1つの集約の中に閉じ込めることを指す。**
閉じ込められるからこそ、ロックで守り切れる。

### 1.3 集約をまたぐ操作

**注文と在庫をまたぐ操作（引当を伴う注文作成）は、ひとつのDBトランザクションで実行する。**
将来サービス分割する場合はSagaに置き換える余地を残すが、フェーズ1では単一DBの強整合を選ぶ。
判断の経緯は [ADR-0002](adr/0002-inventory-concurrency.md) を参照。

## 2. Product 集約

```
Product
├─ id: ProductID
├─ name: string
├─ description: string
├─ categoryID: CategoryID
├─ status: active | suspended        // suspended は新規注文を受け付けない
└─ skus: []SKU (1..*)
      ├─ id: SKUID
      ├─ code: SKUCode               // 全体で一意。例 "MM-TOWEL-BL-M"
      ├─ price: Money                // 日本円。税抜
      ├─ attributes: map[string]string  // {"color":"blue","size":"M"}
      └─ status: active | suspended
```

- `Product.status = suspended` にすると、配下の全SKUが実質的に販売停止になる
- SKU単体の販売停止も可能（在庫はあるが売らない、というケース）

## 3. Inventory 集約

```
InventoryItem                        // 一意キー: (skuID, locationID)
├─ skuID: SKUID
├─ locationID: LocationID
├─ onHand: int                       // 実在庫数
├─ reserved: int                      // 引当済数
└─ version: int                       // 楽観ロック用

  available() = onHand - reserved     // 引当可能数
```

### 不変条件

1. `onHand >= 0`
2. `reserved >= 0`
3. `reserved <= onHand` — 引当済が実在庫を超えることはない
4. `available() >= 0` — 3 から自動的に導かれる

### 状態を変える操作

| 操作 | 前提 | onHand | reserved | 発生タイミング |
| --- | --- | --- | --- | --- |
| `Reserve(qty)` | `available() >= qty` | 変化なし | `+qty` | 注文確定 (FR-202) |
| `Release(qty)` | `reserved >= qty` | 変化なし | `-qty` | 注文キャンセル (FR-203) |
| `Ship(qty)` | `reserved >= qty` | `-qty` | `-qty` | 出荷完了 (FR-204) |
| `Receive(qty)` | `qty > 0` | `+qty` | 変化なし | 入荷 (FR-205) |
| `Adjust(delta, reason)` | 結果が不変条件を満たす | `+delta` | 変化なし | 棚卸補正 (FR-206) |

すべての操作は `InventoryMovement`（在庫変動履歴）を1件生成する。履歴は追記のみで、更新・削除しない。

### 具体例

SKU `MM-TOWEL-BL-M` が拠点 `WH-TOKYO` に 実在庫10・引当0 で存在するとき:

```
初期状態         onHand=10, reserved=0,  available=10
注文A(3個)引当 → onHand=10, reserved=3,  available=7
注文B(8個)引当 → available(7) < 8 なので失敗 ★ここで売り越しを防ぐ
注文A出荷完了  → onHand=7,  reserved=0,  available=7
棚卸で1個不足  → onHand=6,  reserved=0,  available=6  (reason: STOCKTAKING_LOSS)
```

## 4. Order 集約

```
Order
├─ id: OrderID
├─ orderNumber: string                // 顧客提示用。例 "MM-20260601-000123"
├─ channelCode: ChannelCode           // 設定で追加可能 (FR-401)
├─ locationID: LocationID             // 単一拠点で完結する
├─ status: OrderStatus
├─ customer: CustomerInfo             // 氏名/電話/メール ※個人情報
├─ shippingAddress: Address           // ※個人情報
├─ idempotencyKey: string             // 冪等キー (FR-302)
├─ lines: []OrderLine (1..*)
│     ├─ skuID / skuCode              // skuCodeは注文時点の値を保持（スナップショット）
│     ├─ quantity: int
│     └─ unitPrice: Money             // 注文時点の価格を保持
├─ totalAmount: Money                 // = Σ(unitPrice × quantity)
└─ history: []StatusChange            // 追記のみ (FR-309)
      ├─ from / to: OrderStatus
      ├─ changedBy: ActorID
      ├─ changedAt: time
      └─ reason: string
```

**価格とSKUコードは注文時点の値をコピーして保持する。** マスタが後から変わっても過去の注文は変わらない。

### 状態遷移

```mermaid
stateDiagram-v2
  [*] --> Pending: 注文登録(在庫引当済)
  Pending --> Allocated: 出荷指示
  Allocated --> Shipped: 出荷完了(出庫確定)
  Shipped --> [*]
  Pending --> Cancelled: キャンセル(引当解除)
  Allocated --> Cancelled: キャンセル(引当解除)
  Cancelled --> [*]
```

| 状態 | 意味 | 在庫の状態 |
| --- | --- | --- |
| `pending` | 受付済。出荷準備待ち | 引当済 |
| `allocated` | 出荷指示済。WMSへ連携済 | 引当済 |
| `shipped` | 出荷完了 | 出庫確定（実在庫から減算済） |
| `cancelled` | キャンセル | 引当解除済 |

**`shipped` からの遷移は無い。** 返品はフェーズ1のスコープ外で、入荷登録(FR-205)で吸収する。

### 遷移の禁止事項

- `shipped` → `cancelled` は不可（出荷済のものはキャンセルできない）
- `cancelled` からのすべての遷移は不可
- 同じ状態への遷移は不可（`pending` → `pending` はエラー）

## 5. ドメインイベント

フェーズ1では同一プロセス内のハンドラで処理するが、将来の非同期化に備えて名前を定義しておく。

| イベント | 発生元 | 用途 |
| --- | --- | --- |
| `OrderPlaced` | Order | 監査ログ、モール側への在庫連携 |
| `OrderCancelled` | Order | 監査ログ、引当解除 |
| `OrderShipped` | Order | 監査ログ、出庫確定 |
| `StockLow` | Inventory | 在庫アラート (FR-208) |

## 6. ユビキタス言語（コード上の名前）

要件定義の用語をコードでどう表記するかを固定する。

| 日本語 | コード上の名前 | 使ってはいけない別名 |
| --- | --- | --- |
| 実在庫数 | `OnHand` | `stock`, `quantity` |
| 引当済数 | `Reserved` | `allocated`, `held` |
| 引当可能数 | `Available` | `free`, `sellable` |
| 引当 | `Reserve` | `allocate`, `hold` |
| 引当解除 | `Release` | `unreserve`, `cancel` |
| 出庫確定 | `Ship` | `dispatch`, `commit` |
| 拠点 | `Location` | `warehouse`, `site` |
| チャネル | `Channel` | `source`, `marketplace` |

「引当」に `allocate` を使わないのは、注文ステータスの `allocated`（出荷指示済）と紛らわしいため。
