-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Copyright (c) 2026 ShangBin Wang

-- Migration: documents, document_lines, inventory_transactions

-- 单据主表 / Documents
CREATE TABLE IF NOT EXISTS documents (
    id              BIGSERIAL    PRIMARY KEY,
    doc_no          VARCHAR(32)  NOT NULL,
    doc_type        SMALLINT     NOT NULL,
    status          SMALLINT     NOT NULL DEFAULT 1,
    warehouse_id    BIGINT       NOT NULL REFERENCES warehouses(id),
    to_warehouse_id BIGINT       NOT NULL DEFAULT 0,
    ref_doc_id      BIGINT       NOT NULL DEFAULT 0,
    ref_doc_no      VARCHAR(32)  NOT NULL DEFAULT '',
    remark          VARCHAR(512) NOT NULL DEFAULT '',
    operated_at     TIMESTAMPTZ,
    created_by      BIGINT       NOT NULL DEFAULT 0,
    updated_by      BIGINT       NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_documents_doc_no ON documents(doc_no) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_documents_doc_type ON documents(doc_type);
CREATE INDEX IF NOT EXISTS idx_documents_status ON documents(status);
CREATE INDEX IF NOT EXISTS idx_documents_warehouse_id ON documents(warehouse_id);
CREATE INDEX IF NOT EXISTS idx_documents_ref_doc_id ON documents(ref_doc_id) WHERE ref_doc_id > 0;
CREATE INDEX IF NOT EXISTS idx_documents_deleted_at ON documents(deleted_at);
CREATE INDEX IF NOT EXISTS idx_documents_created_at ON documents(created_at);

-- 单据明细表 / Document Lines
CREATE TABLE IF NOT EXISTS document_lines (
    id             BIGSERIAL      PRIMARY KEY,
    document_id    BIGINT         NOT NULL REFERENCES documents(id),
    product_id     BIGINT         NOT NULL DEFAULT 0,
    non_std_inv_id BIGINT         NOT NULL DEFAULT 0,
    product_code   VARCHAR(32)    NOT NULL DEFAULT '',
    product_name   VARCHAR(128)   NOT NULL DEFAULT '',
    quantity       INT            NOT NULL DEFAULT 0,
    unit           VARCHAR(16)    NOT NULL DEFAULT '',
    cost_price     DECIMAL(12,2)  NOT NULL DEFAULT 0,
    retail_price   DECIMAL(12,2)  NOT NULL DEFAULT 0,
    system_qty     INT            NOT NULL DEFAULT 0,
    actual_qty     INT            NOT NULL DEFAULT 0,
    diff_qty       INT            NOT NULL DEFAULT 0,
    remark         VARCHAR(255)   NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    deleted_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_doc_lines_document_id ON document_lines(document_id);
CREATE INDEX IF NOT EXISTS idx_doc_lines_product_id ON document_lines(product_id) WHERE product_id > 0;
CREATE INDEX IF NOT EXISTS idx_doc_lines_deleted_at ON document_lines(deleted_at);

-- 库存流水表 / Inventory Transactions
CREATE TABLE IF NOT EXISTS inventory_transactions (
    id            BIGSERIAL    PRIMARY KEY,
    warehouse_id  BIGINT       NOT NULL REFERENCES warehouses(id),
    product_id    BIGINT       NOT NULL,
    doc_id        BIGINT       NOT NULL REFERENCES documents(id),
    doc_no        VARCHAR(32)  NOT NULL,
    doc_type      SMALLINT     NOT NULL,
    direction     SMALLINT     NOT NULL,
    quantity      INT          NOT NULL,
    before_qty    INT          NOT NULL,
    after_qty     INT          NOT NULL,
    operator_id   BIGINT       NOT NULL,
    operated_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at    TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_inv_txn_warehouse_id ON inventory_transactions(warehouse_id);
CREATE INDEX IF NOT EXISTS idx_inv_txn_product_id ON inventory_transactions(product_id);
CREATE INDEX IF NOT EXISTS idx_inv_txn_doc_id ON inventory_transactions(doc_id);
CREATE INDEX IF NOT EXISTS idx_inv_txn_doc_type ON inventory_transactions(doc_type);
CREATE INDEX IF NOT EXISTS idx_inv_txn_deleted_at ON inventory_transactions(deleted_at);
CREATE INDEX IF NOT EXISTS idx_inv_txn_operated_at ON inventory_transactions(operated_at);
