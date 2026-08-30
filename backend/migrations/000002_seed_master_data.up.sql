-- 開発・検証用のマスタデータ。
-- 本番では channels / locations / actors のみを投入し、商品と在庫は移行スクリプト(MIN-050)で入れる。

INSERT INTO locations (id, code, name) VALUES
    ('00000000-0000-0000-0000-000000000101', 'WH-TOKYO', '自社倉庫（東京）'),
    ('00000000-0000-0000-0000-000000000102', 'WH-3PL-OSAKA', '3PL倉庫（大阪）');

INSERT INTO channels (id, code, name) VALUES
    ('00000000-0000-0000-0000-000000000201', 'EC_OWN', '自社ECサイト'),
    ('00000000-0000-0000-0000-000000000202', 'RAKUTEN', '楽天市場'),
    ('00000000-0000-0000-0000-000000000203', 'AMAZON', 'Amazon');

-- パスワードはいずれも 'password123' のbcryptハッシュ（開発用のみ）。
INSERT INTO actors (id, type, email, password_hash, name, role) VALUES
    ('00000000-0000-0000-0000-000000000301', 'staff',
     'admin@minatomart.example', '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy',
     '管理者 太郎', 'admin'),
    ('00000000-0000-0000-0000-000000000302', 'staff',
     'operator@minatomart.example', '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy',
     'オペレーター 花子', 'operator');

INSERT INTO actors (id, type, name, role) VALUES
    ('00000000-0000-0000-0000-000000000311', 'api_client', 'ECサイト連携', 'operator'),
    ('00000000-0000-0000-0000-000000000312', 'api_client', 'モール連携バッチ', 'operator');

INSERT INTO categories (id, name) VALUES
    ('00000000-0000-0000-0000-000000000401', 'タオル・バス用品'),
    ('00000000-0000-0000-0000-000000000402', 'キッチン用品');

INSERT INTO products (id, name, description, category_id) VALUES
    ('00000000-0000-0000-0000-000000000501', 'today''s タオル',
     '毎日使いたくなる today''s シリーズのフェイスタオル',
     '00000000-0000-0000-0000-000000000401'),
    ('00000000-0000-0000-0000-000000000502', 'シンプルマグ',
     '電子レンジ・食洗機対応の陶器マグ',
     '00000000-0000-0000-0000-000000000402');

INSERT INTO skus (id, product_id, code, price_amount, attributes) VALUES
    ('00000000-0000-0000-0000-000000000601',
     '00000000-0000-0000-0000-000000000501', 'MM-TOWEL-BL-M', 1480,
     '{"color": "blue", "size": "M"}'),
    ('00000000-0000-0000-0000-000000000602',
     '00000000-0000-0000-0000-000000000501', 'MM-TOWEL-GY-M', 1480,
     '{"color": "gray", "size": "M"}'),
    ('00000000-0000-0000-0000-000000000603',
     '00000000-0000-0000-0000-000000000502', 'MM-MUG-WH-L', 1980,
     '{"color": "white", "size": "L"}');

-- 在庫。docs/02-domain-model.md 3章の例と同じく MM-TOWEL-BL-M は東京に10個。
INSERT INTO inventory_items (sku_id, location_id, on_hand, reserved) VALUES
    ('00000000-0000-0000-0000-000000000601', '00000000-0000-0000-0000-000000000101', 10, 0),
    ('00000000-0000-0000-0000-000000000601', '00000000-0000-0000-0000-000000000102',  5, 0),
    ('00000000-0000-0000-0000-000000000602', '00000000-0000-0000-0000-000000000101', 30, 0),
    ('00000000-0000-0000-0000-000000000603', '00000000-0000-0000-0000-000000000101', 25, 0);
