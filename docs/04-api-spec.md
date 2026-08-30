# API設計

機械可読な契約は [`api/openapi.yaml`](../api/openapi.yaml) が正。本書は設計方針と一覧を扱う。

## 1. 方針

| 項目 | 決定 |
| --- | --- |
| スタイル | REST。リソース指向。ただし状態遷移は動詞のサブリソースで表す |
| ベースURL | `/api/v1` |
| 形式 | JSON。キーは `lowerCamelCase` |
| 日時 | RFC 3339、UTC、ミリ秒まで（`2026-06-01T09:30:00.000Z`） |
| 金額 | 整数の円。`{"amount": 1980, "currency": "JPY"}` |
| ID | UUID v7（時系列ソート可能なため） |
| ページング | `?page=1&perPage=50`。レスポンスに `pagination` を含める |
| バージョニング | URLパス。破壊的変更時のみ `/api/v2` を切る |

### 状態遷移をどう表現するか

`PATCH /orders/{id}` で `status` を書き換える形にはしない。
理由は、遷移ごとに必要な入力が違う（キャンセルには理由が要る、出荷完了には追跡番号が要る）ことと、
「どの遷移を実行したか」がログとアクセス制御の単位として意味を持つため。

```
POST /orders/{id}/allocate    出荷指示
POST /orders/{id}/ship        出荷完了
POST /orders/{id}/cancel      キャンセル（reason 必須）
```

## 2. エンドポイント一覧

### 商品

| メソッド | パス | 機能 | 要件 | 権限 |
| --- | --- | --- | --- | --- |
| GET | `/api/v1/products` | 商品一覧・検索 | FR-103 | operator |
| POST | `/api/v1/products` | 商品登録 | FR-101 | admin |
| GET | `/api/v1/products/{id}` | 商品詳細（SKU・在庫込み） | FR-104 | operator |
| PATCH | `/api/v1/products/{id}` | 商品更新 | FR-101 | admin |
| POST | `/api/v1/products/{id}/suspend` | 販売停止 | FR-105 | admin |
| POST | `/api/v1/products/{id}/skus` | SKU追加 | FR-102 | admin |

### 在庫

| メソッド | パス | 機能 | 要件 | 権限 |
| --- | --- | --- | --- | --- |
| GET | `/api/v1/inventory` | 在庫一覧（SKU・拠点で絞込） | FR-201 | operator |
| GET | `/api/v1/inventory/{skuId}/{locationId}` | 在庫照会 | FR-201 | operator |
| POST | `/api/v1/inventory/{skuId}/{locationId}/receive` | 入荷登録 | FR-205 | operator |
| POST | `/api/v1/inventory/{skuId}/{locationId}/adjust` | 在庫補正 | FR-206 | **admin** |
| GET | `/api/v1/inventory/{skuId}/{locationId}/movements` | 変動履歴 | FR-207 | operator |
| GET | `/api/v1/inventory/alerts` | 在庫アラート一覧 | FR-208 | operator |

**引当・引当解除の単独APIは公開しない。** 在庫の引当は注文操作の副作用としてのみ発生する。
APIとして引当だけを許すと、注文と紐づかない引当が作れてしまい、追跡不能な在庫が生まれる。

### 注文

| メソッド | パス | 機能 | 要件 | 権限 |
| --- | --- | --- | --- | --- |
| POST | `/api/v1/orders` | 注文登録（引当込み・冪等） | FR-301,302 | api_client |
| GET | `/api/v1/orders` | 注文検索 | FR-303 | operator |
| GET | `/api/v1/orders/{id}` | 注文詳細 | FR-304 | operator |
| POST | `/api/v1/orders/{id}/allocate` | 出荷指示 | FR-305 | operator |
| POST | `/api/v1/orders/{id}/ship` | 出荷完了 | FR-306 | operator/api_client |
| POST | `/api/v1/orders/{id}/cancel` | キャンセル | FR-307 | operator |
| GET | `/api/v1/orders/{id}/history` | 状態遷移履歴 | FR-309 | operator |

### 認証・その他

| メソッド | パス | 機能 | 要件 |
| --- | --- | --- | --- |
| POST | `/api/v1/auth/login` | 職員ログイン | FR-501 |
| POST | `/api/v1/auth/refresh` | トークン更新 | FR-501 |
| GET | `/api/v1/channels` | チャネル一覧 | FR-401 |
| GET | `/api/v1/locations` | 拠点一覧 | — |
| GET | `/healthz` | 死活監視（認証不要） | NFR-04 |
| GET | `/readyz` | DB接続を含む準備状態（認証不要） | NFR-04 |
| GET | `/metrics` | Prometheusメトリクス | NFR-07 |

## 3. 注文登録のリクエスト／レスポンス

```http
POST /api/v1/orders
Authorization: Bearer <api-key>
Idempotency-Key: 018f4a2b-9c1d-7e3f-8a5b-1c2d3e4f5a6b
Content-Type: application/json

{
  "channelCode": "EC_OWN",
  "locationCode": "WH-TOKYO",
  "customer": {
    "name": "山田 太郎",
    "email": "yamada@example.com",
    "phone": "090-1234-5678"
  },
  "shippingAddress": {
    "postalCode": "150-0001",
    "prefecture": "東京都",
    "city": "渋谷区",
    "line1": "神宮前1-2-3",
    "line2": "サンプルマンション101"
  },
  "lines": [
    { "skuCode": "MM-TOWEL-BL-M", "quantity": 2 },
    { "skuCode": "MM-MUG-WH-L",   "quantity": 1 }
  ]
}
```

成功時 `201 Created`:

```json
{
  "id": "018f4a2c-0000-7000-8000-000000000001",
  "orderNumber": "MM-20260601-000123",
  "status": "pending",
  "channelCode": "EC_OWN",
  "locationCode": "WH-TOKYO",
  "totalAmount": { "amount": 4940, "currency": "JPY" },
  "lines": [
    { "skuCode": "MM-TOWEL-BL-M", "quantity": 2, "unitPrice": { "amount": 1480, "currency": "JPY" } },
    { "skuCode": "MM-MUG-WH-L",   "quantity": 1, "unitPrice": { "amount": 1980, "currency": "JPY" } }
  ],
  "placedAt": "2026-06-01T09:30:00.000Z"
}
```

在庫不足時 `409 Conflict`:

```json
{
  "error": {
    "code": "INSUFFICIENT_STOCK",
    "message": "在庫が不足しています",
    "details": [
      { "skuCode": "MM-TOWEL-BL-M", "requested": 2, "available": 1 }
    ]
  }
}
```

**不足したSKUをすべて返す**（最初の1件で打ち切らない）。
オペレーターが顧客に代替提案する際、何がいくつ足りないか一度でわかる必要があるため。

## 4. 認証方式

| 対象 | 方式 | 有効期限 |
| --- | --- | --- |
| 職員（管理画面） | JWT Bearer。`Authorization: Bearer <token>` | アクセス30分 / リフレッシュ14日 |
| 外部システム | APIキー。`Authorization: Bearer <api-key>` | 無期限（手動失効） |

JWTのクレームに `sub`（actorID）、`role`、`exp` を含める。
権限判定はミドルウェアで行い、ハンドラ内で個別に書かない。

## 5. 個人情報のマスキング（NFR-06）

`operator` ロールは業務上、顧客の氏名・住所・電話番号を参照する必要がある。
そのため注文詳細ではマスクしない。ただし以下を守る。

- **注文一覧（GET /orders）では電話番号とメールアドレスを返さない。** 氏名のみ
- 監査ログには個人情報の値そのものを書かない。`order_id` のみ記録する
- CSVエクスポートは admin のみ

## 6. レート制限

| 対象 | 制限 |
| --- | --- |
| 外部システム（APIキー単位） | 100 req/s。バースト200 |
| 職員（actor単位） | 20 req/s |

超過時は `429 Too Many Requests` と `Retry-After` ヘッダを返す。
セール時のECサイトからの注文は制限に達しうるため、EC_OWN のキーは個別に上限を引き上げる。
