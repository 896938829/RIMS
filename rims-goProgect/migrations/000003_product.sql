-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Copyright (c) 2026 ShangBin Wang
-- Migration: products, inventories, non_std_inventories

-- Products (global catalog)
CREATE TABLE IF NOT EXISTS products (
    id            BIGSERIAL    PRIMARY KEY,
    code          VARCHAR(32)  NOT NULL UNIQUE,
    name          VARCHAR(128) NOT NULL,
    category      VARCHAR(64)  DEFAULT '',
    spec          VARCHAR(128) DEFAULT '',
    unit          VARCHAR(16)  NOT NULL,
    barcode       VARCHAR(64)  DEFAULT '',
    retail_price  DECIMAL(12,2) NOT NULL DEFAULT 0,
    cost_price    DECIMAL(12,2) NOT NULL DEFAULT 0,
    image_url     VARCHAR(512) DEFAULT '',
    status        SMALLINT     NOT NULL DEFAULT 1,
    created_by    BIGINT       NOT NULL DEFAULT 0,
    updated_by    BIGINT       NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at    TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_products_deleted_at ON products(deleted_at);
CREATE INDEX IF NOT EXISTS idx_products_status ON products(status);
CREATE INDEX IF NOT EXISTS idx_products_category ON products(category);
CREATE UNIQUE INDEX IF NOT EXISTS idx_products_barcode
    ON products(barcode) WHERE deleted_at IS NULL AND barcode != '';

-- Standard inventories (per warehouse per product)
CREATE TABLE IF NOT EXISTS inventories (
    id              BIGSERIAL   PRIMARY KEY,
    warehouse_id    BIGINT      NOT NULL REFERENCES warehouses(id),
    product_id      BIGINT      NOT NULL REFERENCES products(id),
    quantity        INT         NOT NULL DEFAULT 0,
    locked_qty      INT         NOT NULL DEFAULT 0,
    alert_threshold INT         NOT NULL DEFAULT 0,
    status          SMALLINT    NOT NULL DEFAULT 1,
    created_by      BIGINT      NOT NULL DEFAULT 0,
    updated_by      BIGINT      NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_inv_wh_product
    ON inventories(warehouse_id, product_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_inventories_warehouse_id ON inventories(warehouse_id);
CREATE INDEX IF NOT EXISTS idx_inventories_product_id ON inventories(product_id);
CREATE INDEX IF NOT EXISTS idx_inventories_status ON inventories(status);
CREATE INDEX IF NOT EXISTS idx_inventories_deleted_at ON inventories(deleted_at);

-- Non-standard inventories (per warehouse, temp items)
CREATE TABLE IF NOT EXISTS non_std_inventories (
    id              BIGSERIAL    PRIMARY KEY,
    warehouse_id    BIGINT       NOT NULL REFERENCES warehouses(id),
    temp_label      VARCHAR(64)  NOT NULL,
    description     VARCHAR(255) NOT NULL,
    unit            VARCHAR(16)  NOT NULL,
    quantity        INT          NOT NULL,
    converted_qty   INT          NOT NULL DEFAULT 0,
    source_method   VARCHAR(32)  DEFAULT '',
    source_document VARCHAR(64)  DEFAULT '',
    status          SMALLINT     NOT NULL DEFAULT 1,
    created_by      BIGINT       NOT NULL DEFAULT 0,
    updated_by      BIGINT       NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_nonstd_temp_label
    ON non_std_inventories(temp_label) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_nonstd_warehouse_id ON non_std_inventories(warehouse_id);
CREATE INDEX IF NOT EXISTS idx_nonstd_status ON non_std_inventories(status);
CREATE INDEX IF NOT EXISTS idx_nonstd_deleted_at ON non_std_inventories(deleted_at);
