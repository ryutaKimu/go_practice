-- 依存の逆順に落とす。down が通ることは必ずローカルで確認する
-- （docs/06-operations.md 4.2 デプロイのチェックリスト）。

DROP TABLE IF EXISTS order_status_changes;
DROP TABLE IF EXISTS order_lines;
DROP TABLE IF EXISTS inventory_movements;
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS inventory_items;
DROP TABLE IF EXISTS skus;
DROP TABLE IF EXISTS products;
DROP TABLE IF EXISTS actors;
DROP TABLE IF EXISTS channels;
DROP TABLE IF EXISTS locations;
DROP TABLE IF EXISTS categories;
