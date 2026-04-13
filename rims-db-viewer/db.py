# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (c) 2026 ShangBin Wang

import os
from pathlib import Path

import psycopg2
from psycopg2 import pool
from dotenv import load_dotenv

# Load .env from project root (parent of rims-db-viewer/)
_env_path = Path(__file__).resolve().parent.parent / ".env"
load_dotenv(_env_path)

_pool = None


def get_pool():
    global _pool
    if _pool is None:
        _pool = pool.SimpleConnectionPool(
            minconn=1,
            maxconn=5,
            host=os.getenv("DB_HOST", "127.0.0.1"),
            port=int(os.getenv("DB_PORT", "5432")),
            user=os.getenv("DB_USER", "app"),
            password=os.getenv("DB_PASSWORD", ""),
            dbname=os.getenv("DB_NAME", "appdb"),
            sslmode=os.getenv("DB_SSLMODE", "disable"),
        )
    return _pool


def get_conn():
    return get_pool().getconn()


def put_conn(conn):
    get_pool().putconn(conn)


def get_tables():
    """Return list of (table_name, estimated_row_count) for public schema."""
    conn = get_conn()
    try:
        with conn.cursor() as cur:
            cur.execute("""
                SELECT t.table_name,
                       GREATEST(COALESCE(c.reltuples, 0)::bigint, 0) AS est_rows
                FROM information_schema.tables t
                LEFT JOIN pg_class c ON c.relname = t.table_name
                WHERE t.table_schema = 'public'
                  AND t.table_type = 'BASE TABLE'
                ORDER BY t.table_name
            """)
            return cur.fetchall()
    finally:
        put_conn(conn)


def get_table_names():
    """Return set of valid table names in public schema."""
    return {row[0] for row in get_tables()}


def get_columns(table_name):
    """Return list of column names for a table."""
    conn = get_conn()
    try:
        with conn.cursor() as cur:
            cur.execute("""
                SELECT column_name
                FROM information_schema.columns
                WHERE table_schema = 'public' AND table_name = %s
                ORDER BY ordinal_position
            """, (table_name,))
            return [row[0] for row in cur.fetchall()]
    finally:
        put_conn(conn)


def get_column_details(table_name):
    """Return detailed column info for structure page."""
    conn = get_conn()
    try:
        with conn.cursor() as cur:
            cur.execute("""
                SELECT column_name, data_type, is_nullable,
                       column_default, character_maximum_length
                FROM information_schema.columns
                WHERE table_schema = 'public' AND table_name = %s
                ORDER BY ordinal_position
            """, (table_name,))
            return cur.fetchall()
    finally:
        put_conn(conn)


def get_indexes(table_name):
    """Return list of (index_name, index_def) for a table."""
    conn = get_conn()
    try:
        with conn.cursor() as cur:
            cur.execute("""
                SELECT indexname, indexdef
                FROM pg_indexes
                WHERE schemaname = 'public' AND tablename = %s
                ORDER BY indexname
            """, (table_name,))
            return cur.fetchall()
    finally:
        put_conn(conn)


def query_table(table_name, columns, filters=None, sort_col=None,
                sort_dir="ASC", page=1, per_page=50):
    """Query table data with filtering, sorting, pagination.

    Returns (rows, total_count).
    filters: dict of {column_name: search_text}
    """
    conn = get_conn()
    try:
        with conn.cursor() as cur:
            where_clauses = ["TRUE"]
            params = []

            if filters:
                for col, val in filters.items():
                    if val and col in columns:
                        where_clauses.append(
                            f'"{col}"::text ILIKE %s'
                        )
                        params.append(f"%{val}%")

            where_sql = " AND ".join(where_clauses)

            # Validate sort column
            order_sql = ""
            if sort_col and sort_col in columns:
                direction = "DESC" if sort_dir.upper() == "DESC" else "ASC"
                order_sql = f'ORDER BY "{sort_col}" {direction} NULLS LAST'
            else:
                order_sql = "ORDER BY 1"

            offset = (page - 1) * per_page

            # Count query
            cur.execute(
                f'SELECT COUNT(*) FROM "{table_name}" WHERE {where_sql}',
                params,
            )
            total = cur.fetchone()[0]

            # Data query
            cur.execute(
                f'SELECT * FROM "{table_name}" WHERE {where_sql} '
                f'{order_sql} LIMIT %s OFFSET %s',
                params + [per_page, offset],
            )
            rows = cur.fetchall()

            return rows, total
    finally:
        put_conn(conn)


def query_table_all(table_name, columns, filters=None, sort_col=None,
                    sort_dir="ASC", limit=10000):
    """Query table data for CSV export (no pagination, with limit)."""
    conn = get_conn()
    try:
        with conn.cursor() as cur:
            where_clauses = ["TRUE"]
            params = []

            if filters:
                for col, val in filters.items():
                    if val and col in columns:
                        where_clauses.append(f'"{col}"::text ILIKE %s')
                        params.append(f"%{val}%")

            where_sql = " AND ".join(where_clauses)

            order_sql = ""
            if sort_col and sort_col in columns:
                direction = "DESC" if sort_dir.upper() == "DESC" else "ASC"
                order_sql = f'ORDER BY "{sort_col}" {direction} NULLS LAST'
            else:
                order_sql = "ORDER BY 1"

            cur.execute(
                f'SELECT * FROM "{table_name}" WHERE {where_sql} '
                f'{order_sql} LIMIT %s',
                params + [limit],
            )
            return cur.fetchall()
    finally:
        put_conn(conn)


def execute_readonly_query(sql, limit=1000):
    """Execute a read-only SQL query. Returns (columns, rows, elapsed_ms).

    Raises ValueError if the query is not a SELECT/WITH statement.
    """
    import time

    stripped = sql.strip().rstrip(";").strip()
    upper = stripped.upper()

    if not (upper.startswith("SELECT") or upper.startswith("WITH")):
        raise ValueError("Only SELECT or WITH queries are allowed.")

    # Reject multiple statements
    if ";" in stripped:
        raise ValueError("Multiple statements are not allowed.")

    conn = get_conn()
    try:
        with conn.cursor() as cur:
            cur.execute("SET statement_timeout = '10s'")
            start = time.time()
            cur.execute(f"{stripped} LIMIT {limit}")
            rows = cur.fetchall()
            elapsed = round((time.time() - start) * 1000, 2)
            columns = [desc[0] for desc in cur.description]
            return columns, rows, elapsed
    except Exception:
        conn.rollback()
        raise
    finally:
        try:
            with conn.cursor() as cur:
                cur.execute("RESET statement_timeout")
        except Exception:
            pass
        put_conn(conn)


def execute_readonly_query_all(sql, limit=10000):
    """Execute a read-only SQL query for CSV export."""
    stripped = sql.strip().rstrip(";").strip()
    upper = stripped.upper()

    if not (upper.startswith("SELECT") or upper.startswith("WITH")):
        raise ValueError("Only SELECT or WITH queries are allowed.")

    if ";" in stripped:
        raise ValueError("Multiple statements are not allowed.")

    conn = get_conn()
    try:
        with conn.cursor() as cur:
            cur.execute("SET statement_timeout = '10s'")
            cur.execute(f"{stripped} LIMIT {limit}")
            rows = cur.fetchall()
            columns = [desc[0] for desc in cur.description]
            return columns, rows
    except Exception:
        conn.rollback()
        raise
    finally:
        try:
            with conn.cursor() as cur:
                cur.execute("RESET statement_timeout")
        except Exception:
            pass
        put_conn(conn)
