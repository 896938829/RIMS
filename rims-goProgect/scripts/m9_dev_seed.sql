-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Copyright (c) 2026 ShangBin Wang

BEGIN;

SELECT pg_advisory_xact_lock(908130011);

INSERT INTO warehouses (
    code, name, status, address, contact_person, contact_phone,
    created_by, updated_by, deleted_at
)
VALUES (
    'M9-WH-02', 'M9 验收二号仓', 1, 'M9 local acceptance only',
    'M9 Fixture', '00000000000', 0, 0, NULL
)
ON CONFLICT (code) DO UPDATE SET
    name = EXCLUDED.name,
    status = EXCLUDED.status,
    address = EXCLUDED.address,
    contact_person = EXCLUDED.contact_person,
    contact_phone = EXCLUDED.contact_phone,
    updated_at = NOW(),
    deleted_at = NULL;

INSERT INTO warehouses (code, name, status, created_by, updated_by, deleted_at)
VALUES ('WH001', '默认仓库', 1, 0, 0, NULL)
ON CONFLICT (code) DO UPDATE SET
    status = 1,
    updated_at = NOW(),
    deleted_at = NULL;

WITH chosen AS (
    SELECT id
    FROM users
    WHERE username = 'm9_operator'
    ORDER BY (deleted_at IS NULL) DESC, id
    LIMIT 1
), updated AS (
    UPDATE users AS u
    SET password_hash = '$2a$10$z4rzZyUqSvr52UB56i7JF.i7OwzHrTSVM6ogCnJBsJ4whq7GVMDgy',
        real_name = 'M9 验收操作员',
        phone = '',
        email = '',
        role_id = (SELECT id FROM roles WHERE code = 'user' AND deleted_at IS NULL),
        status = 1,
        updated_at = NOW(),
        deleted_at = NULL
    FROM chosen
    WHERE u.id = chosen.id
    RETURNING u.id
)
INSERT INTO users (
    username, password_hash, real_name, role_id, status, created_at, updated_at
)
SELECT
    'm9_operator',
    '$2a$10$z4rzZyUqSvr52UB56i7JF.i7OwzHrTSVM6ogCnJBsJ4whq7GVMDgy',
    'M9 验收操作员',
    (SELECT id FROM roles WHERE code = 'user' AND deleted_at IS NULL),
    1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM updated);

UPDATE user_warehouses
SET is_default = FALSE,
    updated_at = NOW(),
    deleted_at = NOW()
WHERE user_id = (
        SELECT id FROM users
        WHERE username = 'm9_operator' AND deleted_at IS NULL
    )
  AND warehouse_id NOT IN (
        SELECT id FROM warehouses
        WHERE code IN ('WH001', 'M9-WH-02') AND deleted_at IS NULL
    )
  AND deleted_at IS NULL;

WITH desired AS (
    SELECT u.id AS user_id, w.id AS warehouse_id,
           CASE WHEN w.code = 'WH001' THEN TRUE ELSE FALSE END AS is_default
    FROM users AS u
    CROSS JOIN warehouses AS w
    WHERE u.username = 'm9_operator' AND u.deleted_at IS NULL
      AND w.code IN ('WH001', 'M9-WH-02') AND w.deleted_at IS NULL
    UNION ALL
    SELECT u.id, w.id, CASE WHEN w.code = 'WH001' THEN TRUE ELSE FALSE END
    FROM users AS u
    CROSS JOIN warehouses AS w
    WHERE u.username = 'admin' AND u.deleted_at IS NULL
      AND w.code IN ('WH001', 'M9-WH-02') AND w.deleted_at IS NULL
), chosen AS (
    SELECT desired.*,
           existing.id AS existing_id
    FROM desired
    LEFT JOIN LATERAL (
        SELECT uw.id
        FROM user_warehouses AS uw
        WHERE uw.user_id = desired.user_id
          AND uw.warehouse_id = desired.warehouse_id
        ORDER BY (uw.deleted_at IS NULL) DESC, uw.id
        LIMIT 1
    ) AS existing ON TRUE
), updated AS (
    UPDATE user_warehouses AS uw
    SET is_default = chosen.is_default,
        updated_at = NOW(),
        deleted_at = NULL
    FROM chosen
    WHERE uw.id = chosen.existing_id
    RETURNING uw.id
)
INSERT INTO user_warehouses (
    user_id, warehouse_id, is_default, created_at, updated_at
)
SELECT user_id, warehouse_id, is_default, NOW(), NOW()
FROM chosen
WHERE existing_id IS NULL;

INSERT INTO products (
    code, name, category, spec, unit, barcode,
    retail_price, cost_price, image_url, status,
    created_by, updated_by, deleted_at
)
SELECT
    format('M9-PAGE-%s', to_char(n, 'FM0000')),
    format('M9 分页商品 %s', to_char(n, 'FM0000')),
    CASE WHEN n <= 15 THEN 'M9 食品' WHEN n <= 30 THEN 'M9 日用' ELSE 'M9 饮料' END,
    format('%s 件/箱', 6 + (n % 7)),
    '件',
    CASE n
        WHEN 1 THEN 'M10-ACTIVE-001'
        WHEN 2 THEN 'M10-DISABLED-001'
        WHEN 3 THEN 'M10-WH001-ONLY-001'
        WHEN 4 THEN 'M10-PRODUCT-IMAGE-001'
        ELSE format('6909000%s', to_char(n, 'FM000000'))
    END,
    (10 + n * 0.75)::DECIMAL(12, 2),
    (6 + n * 0.45)::DECIMAL(12, 2),
    '', CASE WHEN n = 2 THEN 0 ELSE 1 END, 0, 0, NULL
FROM generate_series(1, 45) AS series(n)
ON CONFLICT (code) DO UPDATE SET
    name = EXCLUDED.name,
    category = EXCLUDED.category,
    spec = EXCLUDED.spec,
    unit = EXCLUDED.unit,
    barcode = EXCLUDED.barcode,
    retail_price = EXCLUDED.retail_price,
    cost_price = EXCLUDED.cost_price,
    image_url = EXCLUDED.image_url,
    status = EXCLUDED.status,
    updated_at = NOW(),
    deleted_at = NULL;

WITH desired AS (
    SELECT
        w.id AS warehouse_id,
        p.id AS product_id,
        CASE WHEN n <= 5 THEN 2 + CASE WHEN w.code = 'M9-WH-02' THEN 10 ELSE 0 END ELSE 20 + n + CASE WHEN w.code = 'M9-WH-02' THEN 10 ELSE 0 END END AS quantity,
        5 AS alert_threshold,
        CASE
            WHEN n = 3 AND w.code = 'M9-WH-02' THEN 0
            ELSE 1
        END AS inventory_status
    FROM generate_series(1, 45) AS series(n)
    JOIN products AS p
      ON p.code = format('M9-PAGE-%s', to_char(n, 'FM0000'))
    CROSS JOIN warehouses AS w
    WHERE w.code IN ('WH001', 'M9-WH-02')
      AND w.deleted_at IS NULL
), chosen AS (
    SELECT desired.*,
           existing.id AS existing_id
    FROM desired
    LEFT JOIN LATERAL (
        SELECT i.id
        FROM inventories AS i
        WHERE i.warehouse_id = desired.warehouse_id
          AND i.product_id = desired.product_id
        ORDER BY (i.deleted_at IS NULL) DESC, i.id
        LIMIT 1
    ) AS existing ON TRUE
), updated AS (
    UPDATE inventories AS i
    SET quantity = chosen.quantity,
        locked_qty = 0,
        alert_threshold = chosen.alert_threshold,
        status = chosen.inventory_status,
        updated_by = 0,
        updated_at = NOW(),
        deleted_at = NULL
    FROM chosen
    WHERE i.id = chosen.existing_id
    RETURNING i.id
)
INSERT INTO inventories (
    warehouse_id, product_id, quantity, locked_qty, alert_threshold,
    status, created_by, updated_by, created_at, updated_at
)
SELECT
    warehouse_id, product_id, quantity, 0, alert_threshold,
    inventory_status, 0, 0, NOW(), NOW()
FROM chosen
WHERE existing_id IS NULL;

UPDATE non_std_inventories
SET status = 2,
    updated_at = NOW(),
    deleted_at = NOW()
WHERE temp_label LIKE 'M9-NS-%'
  AND temp_label NOT IN (
      SELECT format('M9-NS-%s', to_char(n, 'FM0000'))
      FROM generate_series(1, 25) AS series(n)
  )
  AND deleted_at IS NULL;

WITH desired AS (
    SELECT
        w.id AS warehouse_id,
        format('M9-NS-%s', to_char(n, 'FM0000')) AS temp_label,
        format('M9 非标验收物料 %s', to_char(n, 'FM0000')) AS description,
        8 + n AS quantity
    FROM generate_series(1, 25) AS series(n)
    CROSS JOIN warehouses AS w
    WHERE w.code = 'M9-WH-02' AND w.deleted_at IS NULL
), chosen AS (
    SELECT desired.*,
           existing.id AS existing_id
    FROM desired
    LEFT JOIN LATERAL (
        SELECT nsi.id
        FROM non_std_inventories AS nsi
        WHERE nsi.temp_label = desired.temp_label
        ORDER BY (nsi.deleted_at IS NULL) DESC, nsi.id
        LIMIT 1
    ) AS existing ON TRUE
), updated AS (
    UPDATE non_std_inventories AS nsi
    SET warehouse_id = chosen.warehouse_id,
        description = chosen.description,
        unit = '件',
        quantity = chosen.quantity,
        converted_qty = 0,
        source_method = 'M9 fixture',
        source_document = '',
        status = 1,
        updated_by = 0,
        updated_at = NOW(),
        deleted_at = NULL
    FROM chosen
    WHERE nsi.id = chosen.existing_id
    RETURNING nsi.id
)
INSERT INTO non_std_inventories (
    warehouse_id, temp_label, description, unit, quantity, converted_qty,
    source_method, source_document, status, created_by, updated_by,
    created_at, updated_at
)
SELECT
    warehouse_id, temp_label, description, '件', quantity, 0,
    'M9 fixture', '', 1, 0, 0, NOW(), NOW()
FROM chosen
WHERE existing_id IS NULL;

DELETE FROM inventory_transactions
WHERE doc_no LIKE 'M9DOC%';

DELETE FROM document_lines
WHERE document_id IN (
    SELECT id FROM documents WHERE doc_no LIKE 'M9DOC%'
);

WITH ranked AS (
    SELECT id,
           row_number() OVER (
               PARTITION BY doc_no
               ORDER BY (deleted_at IS NULL) DESC, id
           ) AS row_num
    FROM documents
    WHERE doc_no LIKE 'M9DOC%'
)
DELETE FROM documents AS d
USING ranked
WHERE d.id = ranked.id
  AND ranked.row_num > 1;

DELETE FROM documents
WHERE doc_no LIKE 'M9DOC%'
  AND doc_no NOT IN (
      SELECT format('M9DOC%s', to_char(n, 'FM0000'))
      FROM generate_series(1, 15) AS series(n)
  );

WITH desired AS (
    SELECT
        format('M9DOC%s', to_char(n, 'FM0000')) AS doc_no,
        CASE WHEN n % 2 = 0 THEN wh2.id ELSE wh1.id END AS warehouse_id,
        admin.id AS operator_id,
        CASE n
            WHEN 1 THEN 'M10 attachment target WH001'
            WHEN 2 THEN 'M10 attachment target M9-WH-02'
            ELSE 'M9 fixture read-only document'
        END AS remark,
        n
    FROM generate_series(1, 15) AS series(n)
    CROSS JOIN LATERAL (
        SELECT id FROM warehouses WHERE code = 'WH001' AND deleted_at IS NULL
    ) AS wh1
    CROSS JOIN LATERAL (
        SELECT id FROM warehouses WHERE code = 'M9-WH-02' AND deleted_at IS NULL
    ) AS wh2
    CROSS JOIN LATERAL (
        SELECT id FROM users WHERE username = 'admin' AND deleted_at IS NULL
    ) AS admin
), chosen AS (
    SELECT desired.*,
           existing.id AS existing_id
    FROM desired
    LEFT JOIN LATERAL (
        SELECT d.id
        FROM documents AS d
        WHERE d.doc_no = desired.doc_no
        ORDER BY (d.deleted_at IS NULL) DESC, d.id
        LIMIT 1
    ) AS existing ON TRUE
), updated AS (
    UPDATE documents AS d
    SET doc_type = 2,
        status = 2,
        warehouse_id = chosen.warehouse_id,
        to_warehouse_id = 0,
        ref_doc_id = 0,
        ref_doc_no = '',
        remark = chosen.remark,
        operated_at = NOW() - ((16 - chosen.n) || ' days')::INTERVAL,
        updated_by = chosen.operator_id,
        updated_at = NOW(),
        deleted_at = NULL
    FROM chosen
    WHERE d.id = chosen.existing_id
    RETURNING d.id
)
INSERT INTO documents (
    doc_no, doc_type, status, warehouse_id, to_warehouse_id,
    ref_doc_id, ref_doc_no, remark, operated_at,
    created_by, updated_by, created_at, updated_at
)
SELECT
    doc_no, 2, 2, warehouse_id, 0,
    0, '', remark,
    NOW() - ((16 - n) || ' days')::INTERVAL,
    operator_id, operator_id,
    NOW() - ((16 - n) || ' days')::INTERVAL,
    NOW()
FROM chosen
WHERE existing_id IS NULL;

INSERT INTO document_lines (
    document_id, product_id, product_code, product_name,
    quantity, unit, cost_price, retail_price,
    system_qty, actual_qty, diff_qty, remark,
    created_at, updated_at
)
SELECT
    d.id, p.id, p.code, p.name,
    1 + series.n, p.unit, p.cost_price, p.retail_price,
    0, 0, 0, 'M9 fixture line',
    d.operated_at, d.operated_at
FROM generate_series(1, 15) AS series(n)
JOIN documents AS d
  ON d.doc_no = format('M9DOC%s', to_char(series.n, 'FM0000'))
 AND d.deleted_at IS NULL
JOIN products AS p
  ON p.code = format('M9-PAGE-%s', to_char(series.n, 'FM0000'))
 AND p.deleted_at IS NULL;

INSERT INTO inventory_transactions (
    warehouse_id, product_id, doc_id, doc_no, doc_type,
    direction, quantity, before_qty, after_qty, operator_id,
    operated_at, created_at, updated_at
)
SELECT
    d.warehouse_id, p.id, d.id, d.doc_no, 2,
    -1, 1 + series.n, 100 + series.n, 99,
    (SELECT id FROM users WHERE username = 'admin' AND deleted_at IS NULL),
    d.operated_at, d.operated_at, d.operated_at
FROM generate_series(1, 15) AS series(n)
JOIN documents AS d
  ON d.doc_no = format('M9DOC%s', to_char(series.n, 'FM0000'))
 AND d.deleted_at IS NULL
JOIN products AS p
  ON p.code = format('M9-PAGE-%s', to_char(series.n, 'FM0000'))
 AND p.deleted_at IS NULL;

COMMIT;
