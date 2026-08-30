-- 初版スキーマ。設計は docs/03-erd.md を参照。
-- 命名規則: テーブルは複数形スネークケース、外部キーは <単数形>_id。

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ---------------------------------------------------------------------------
-- マスタ
-- ---------------------------------------------------------------------------

CREATE TABLE categories (
    id        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name      text NOT NULL,
    parent_id uuid REFERENCES categories (id)
);

CREATE TABLE locations (
    code      text NOT NULL UNIQUE,   -- WH-TOKYO 等
    id        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name      text NOT NULL,
    is_active boolean NOT NULL DEFAULT true
);

-- チャネルはテーブルで管理する。新チャネル追加をコード変更なしで行うため(R-04 / FR-401)。
CREATE TABLE channels (
    id        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code      text NOT NULL UNIQUE,   -- EC_OWN / RAKUTEN / AMAZON
    name      text NOT NULL,
    is_active boolean NOT NULL DEFAULT true
);

CREATE TABLE actors (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    type          text NOT NULL CHECK (type IN ('staff', 'api_client')),
    email         text UNIQUE,        -- staff のみ
    password_hash text,               -- staff のみ。bcrypt
    name          text NOT NULL,
    role          text NOT NULL CHECK (role IN ('operator', 'admin')),
    is_active     boolean NOT NULL DEFAULT true,
    created_at    timestamptz NOT NULL DEFAULT now(),
    -- staff はログインするので必ず認証情報を持つ。api_client は持たない。
    CONSTRAINT staff_has_credentials CHECK (
        (type = 'staff' AND email IS NOT NULL AND password_hash IS NOT NULL)
        OR (type = 'api_client' AND email IS NULL AND password_hash IS NULL)
    )
);

-- ---------------------------------------------------------------------------
-- 商品
-- ---------------------------------------------------------------------------

CREATE TABLE products (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name        text NOT NULL,
    description text NOT NULL DEFAULT '',
    category_id uuid NOT NULL REFERENCES categories (id),
    status      text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX products_category_status_idx ON products (category_id, status);

CREATE TABLE skus (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id   uuid NOT NULL REFERENCES products (id) ON DELETE CASCADE,
    code         text NOT NULL UNIQUE,  -- MM-TOWEL-BL-M 等。FR-103 の検索キー
    price_amount bigint NOT NULL CHECK (price_amount >= 0),  -- 円。整数（docs/03-erd.md 2.1）
    attributes   jsonb NOT NULL DEFAULT '{}'::jsonb,          -- {"color":"blue","size":"M"}
    status       text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX skus_product_idx ON skus (product_id);

-- ---------------------------------------------------------------------------
-- 在庫
-- ---------------------------------------------------------------------------

CREATE TABLE inventory_items (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    sku_id      uuid NOT NULL REFERENCES skus (id),
    location_id uuid NOT NULL REFERENCES locations (id),
    on_hand     integer NOT NULL DEFAULT 0,
    reserved    integer NOT NULL DEFAULT 0,
    version     integer NOT NULL DEFAULT 0,
    updated_at  timestamptz NOT NULL DEFAULT now(),

    -- SKU×拠点の在庫が重複すると引当可能数の計算が破綻する。
    CONSTRAINT inventory_items_sku_location_key UNIQUE (sku_id, location_id),

    -- ドメイン層の不変条件をDBにも置く（docs/03-erd.md 2.2）。
    -- アプリを経由しない経路（手動SQL・バッチ）があっても売り越し状態を作れなくする、
    -- R-01 に対する最後の防波堤。
    CONSTRAINT inventory_on_hand_non_negative  CHECK (on_hand >= 0),
    CONSTRAINT inventory_reserved_non_negative CHECK (reserved >= 0),
    CONSTRAINT inventory_no_oversell           CHECK (reserved <= on_hand)
);

CREATE TABLE inventory_movements (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    inventory_item_id uuid NOT NULL REFERENCES inventory_items (id),
    movement_type     text NOT NULL CHECK (
        movement_type IN ('reserve', 'release', 'ship', 'receive', 'adjust')
    ),
    -- 変化量であって変化後の値ではない。積み上げれば現在値になる。
    on_hand_delta   integer NOT NULL DEFAULT 0,
    reserved_delta  integer NOT NULL DEFAULT 0,
    reason_code     text NOT NULL DEFAULT '',
    note            text NOT NULL DEFAULT '',
    order_id        uuid,  -- 引当系のみ。外部キーは orders 作成後に追加する
    actor_id        uuid NOT NULL REFERENCES actors (id),
    occurred_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX inventory_movements_item_time_idx
    ON inventory_movements (inventory_item_id, occurred_at DESC);

-- ---------------------------------------------------------------------------
-- 注文
-- ---------------------------------------------------------------------------

CREATE TABLE orders (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_number text NOT NULL UNIQUE,   -- MM-20260601-000123
    channel_id   uuid NOT NULL REFERENCES channels (id),
    location_id  uuid NOT NULL REFERENCES locations (id),
    status       text NOT NULL CHECK (
        status IN ('pending', 'allocated', 'shipped', 'cancelled')
    ),

    -- 冪等キー。一意制約違反を捕捉して二重注文を防ぐ（ADR-0003）。
    -- 制約名は実装側で判定に使うため、明示的に指定する。
    idempotency_key text NOT NULL,

    -- 個人情報。ログへの出力を禁止する(NFR-06)。
    customer_name    text NOT NULL,
    customer_email   text NOT NULL DEFAULT '',
    customer_phone   text NOT NULL DEFAULT '',
    shipping_address jsonb NOT NULL,

    total_amount bigint NOT NULL CHECK (total_amount >= 0),
    placed_at    timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT orders_idempotency_key_key UNIQUE (idempotency_key)
);

CREATE INDEX orders_status_placed_idx  ON orders (status, placed_at DESC);
CREATE INDEX orders_channel_placed_idx ON orders (channel_id, placed_at DESC);
CREATE INDEX orders_customer_name_idx  ON orders (customer_name text_pattern_ops);

CREATE TABLE order_lines (
    id       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id uuid NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    sku_id   uuid NOT NULL REFERENCES skus (id),

    -- 注文時点のスナップショット。マスタが変わっても過去の注文は変わらない
    -- （docs/03-erd.md 2.5）。冗長だが意図的。
    sku_code   text NOT NULL,
    unit_price bigint NOT NULL CHECK (unit_price >= 0),

    quantity integer NOT NULL CHECK (quantity > 0)
);

CREATE INDEX order_lines_order_idx ON order_lines (order_id);

-- 状態遷移の履歴。追記のみで、UPDATE/DELETE しない(FR-309 / R-02)。
CREATE TABLE order_status_changes (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id    uuid NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    from_status text,   -- 注文作成時は NULL
    to_status   text NOT NULL,
    actor_id    uuid NOT NULL REFERENCES actors (id),
    reason      text NOT NULL DEFAULT '',
    changed_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX order_status_changes_order_idx ON order_status_changes (order_id, changed_at);

-- 在庫変動から注文を辿れるようにする。orders の作成後なのでここで追加。
ALTER TABLE inventory_movements
    ADD CONSTRAINT inventory_movements_order_fk
    FOREIGN KEY (order_id) REFERENCES orders (id);
