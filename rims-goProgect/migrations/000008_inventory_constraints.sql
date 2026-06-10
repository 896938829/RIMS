-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Copyright (c) 2026 ShangBin Wang

-- Migration: inventory integrity constraints

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'chk_inventories_quantity_non_negative'
          AND conrelid = 'inventories'::regclass
    ) THEN
        ALTER TABLE inventories
            ADD CONSTRAINT chk_inventories_quantity_non_negative
            CHECK (quantity >= 0) NOT VALID;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'chk_inventories_locked_qty_non_negative'
          AND conrelid = 'inventories'::regclass
    ) THEN
        ALTER TABLE inventories
            ADD CONSTRAINT chk_inventories_locked_qty_non_negative
            CHECK (locked_qty >= 0) NOT VALID;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'chk_inventories_alert_threshold_non_negative'
          AND conrelid = 'inventories'::regclass
    ) THEN
        ALTER TABLE inventories
            ADD CONSTRAINT chk_inventories_alert_threshold_non_negative
            CHECK (alert_threshold >= 0) NOT VALID;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'chk_non_std_inventories_quantity_non_negative'
          AND conrelid = 'non_std_inventories'::regclass
    ) THEN
        ALTER TABLE non_std_inventories
            ADD CONSTRAINT chk_non_std_inventories_quantity_non_negative
            CHECK (quantity >= 0) NOT VALID;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'chk_non_std_inventories_converted_qty_non_negative'
          AND conrelid = 'non_std_inventories'::regclass
    ) THEN
        ALTER TABLE non_std_inventories
            ADD CONSTRAINT chk_non_std_inventories_converted_qty_non_negative
            CHECK (converted_qty >= 0) NOT VALID;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'chk_non_std_inventories_converted_qty_lte_quantity'
          AND conrelid = 'non_std_inventories'::regclass
    ) THEN
        ALTER TABLE non_std_inventories
            ADD CONSTRAINT chk_non_std_inventories_converted_qty_lte_quantity
            CHECK (converted_qty <= quantity) NOT VALID;
    END IF;
END $$;

ALTER TABLE inventories
    VALIDATE CONSTRAINT chk_inventories_quantity_non_negative;

ALTER TABLE inventories
    VALIDATE CONSTRAINT chk_inventories_locked_qty_non_negative;

ALTER TABLE inventories
    VALIDATE CONSTRAINT chk_inventories_alert_threshold_non_negative;

ALTER TABLE non_std_inventories
    VALIDATE CONSTRAINT chk_non_std_inventories_quantity_non_negative;

ALTER TABLE non_std_inventories
    VALIDATE CONSTRAINT chk_non_std_inventories_converted_qty_non_negative;

ALTER TABLE non_std_inventories
    VALIDATE CONSTRAINT chk_non_std_inventories_converted_qty_lte_quantity;
