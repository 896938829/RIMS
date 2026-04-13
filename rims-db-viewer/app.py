# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (c) 2026 ShangBin Wang

"""RIMS DB Viewer — lightweight database browser for development."""

import csv
import io
import math

from flask import (
    Flask, render_template, request, flash, Response, redirect, url_for,
)

import db

app = Flask(__name__)
app.secret_key = "rims-db-viewer-dev-only"


@app.route("/")
def index():
    """List all tables with estimated row counts."""
    tables = db.get_tables()
    return render_template("index.html", tables=tables)


@app.route("/table/<table_name>")
def table_browse(table_name):
    """Browse table data with filtering, sorting, and pagination."""
    valid_tables = db.get_table_names()
    if table_name not in valid_tables:
        flash(f"Table '{table_name}' not found.", "danger")
        return redirect(url_for("index"))

    columns = db.get_columns(table_name)

    # Parse filters from query params (f_<column>=value)
    filters = {}
    for col in columns:
        val = request.args.get(f"f_{col}", "").strip()
        if val:
            filters[col] = val

    sort_col = request.args.get("sort", "")
    sort_dir = request.args.get("dir", "ASC")
    page = max(1, request.args.get("page", 1, type=int))
    per_page = 50

    rows, total = db.query_table(
        table_name, columns, filters, sort_col, sort_dir, page, per_page,
    )
    total_pages = max(1, math.ceil(total / per_page))

    # Build filter params string for pagination/sort links
    filter_parts = []
    for col in columns:
        val = filters.get(col, "")
        if val:
            filter_parts.append(f"f_{col}={val}")
    filter_params = "&".join(filter_parts)

    # Build full current params for export link
    current_parts = list(filter_parts)
    if sort_col:
        current_parts.append(f"sort={sort_col}")
        current_parts.append(f"dir={sort_dir}")
    current_params = "&".join(current_parts)

    return render_template(
        "table.html",
        table_name=table_name,
        columns=columns,
        rows=rows,
        total=total,
        page=page,
        total_pages=total_pages,
        filters=filters,
        sort_col=sort_col,
        sort_dir=sort_dir,
        filter_params=filter_params,
        current_params=current_params,
    )


@app.route("/table/<table_name>/structure")
def table_structure(table_name):
    """Show table column definitions and indexes."""
    valid_tables = db.get_table_names()
    if table_name not in valid_tables:
        flash(f"Table '{table_name}' not found.", "danger")
        return redirect(url_for("index"))

    column_details = db.get_column_details(table_name)
    indexes = db.get_indexes(table_name)

    return render_template(
        "structure.html",
        table_name=table_name,
        column_details=column_details,
        indexes=indexes,
    )


@app.route("/table/<table_name>/export")
def table_export(table_name):
    """Export table data as CSV with current filters applied."""
    valid_tables = db.get_table_names()
    if table_name not in valid_tables:
        flash(f"Table '{table_name}' not found.", "danger")
        return redirect(url_for("index"))

    columns = db.get_columns(table_name)

    filters = {}
    for col in columns:
        val = request.args.get(f"f_{col}", "").strip()
        if val:
            filters[col] = val

    sort_col = request.args.get("sort", "")
    sort_dir = request.args.get("dir", "ASC")

    rows = db.query_table_all(table_name, columns, filters, sort_col, sort_dir)

    output = io.StringIO()
    writer = csv.writer(output)
    writer.writerow(columns)
    for row in rows:
        writer.writerow(row)

    return Response(
        output.getvalue(),
        mimetype="text/csv",
        headers={
            "Content-Disposition": f"attachment; filename={table_name}_export.csv",
        },
    )


@app.route("/query", methods=["GET", "POST"])
def query_page():
    """SQL query page — GET shows form, POST executes query."""
    sql = ""
    result_columns = None
    result_rows = None
    elapsed_ms = None
    error = None

    if request.method == "POST":
        sql = request.form.get("sql", "").strip()
        if not sql:
            error = "Please enter a SQL query."
        else:
            try:
                result_columns, result_rows, elapsed_ms = (
                    db.execute_readonly_query(sql)
                )
            except ValueError as e:
                error = str(e)
            except Exception as e:
                error = f"Query error: {e}"

    return render_template(
        "query.html",
        sql=sql,
        result_columns=result_columns,
        result_rows=result_rows,
        elapsed_ms=elapsed_ms,
        error=error,
    )


@app.route("/query/export", methods=["POST"])
def query_export():
    """Export SQL query results as CSV."""
    sql = request.form.get("sql", "").strip()
    if not sql:
        flash("Please enter a SQL query.", "danger")
        return redirect(url_for("query_page"))

    try:
        columns, rows = db.execute_readonly_query_all(sql)
    except ValueError as e:
        flash(str(e), "danger")
        return redirect(url_for("query_page"))
    except Exception as e:
        flash(f"Query error: {e}", "danger")
        return redirect(url_for("query_page"))

    output = io.StringIO()
    writer = csv.writer(output)
    writer.writerow(columns)
    for row in rows:
        writer.writerow(row)

    return Response(
        output.getvalue(),
        mimetype="text/csv",
        headers={
            "Content-Disposition": "attachment; filename=query_export.csv",
        },
    )


if __name__ == "__main__":
    app.run(host="0.0.0.0", port=5001, debug=True)
