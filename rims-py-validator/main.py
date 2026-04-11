"""RIMS 前端验证器 CLI 入口。

用法示例：
    python main.py smoke                         # 最小冒烟流程
    python main.py auth login                    # 仅登录
    python main.py auth login -u admin -p xxx    # 覆盖账号
    python main.py document sale                 # （TODO）销售单流程

每个流程都在 `flows/` 下对应一个模块，本文件只负责参数解析、
构建 APIClient、调用业务 run() 并转换异常为退出码。
"""

from __future__ import annotations

import sys
import traceback

import click

from core.client import APIClient, APIError
from core.config import CONFIG
from core.logger import get_logger
from core.session import SESSION

from flows.auth import login as auth_login_flow
from flows.audit import query_logs as audit_query_logs_flow
from flows.document import inbound as doc_inbound_flow
from flows.document import return_flow as doc_return_flow
from flows.document import sale as doc_sale_flow
from flows.document import stocktake as doc_stocktake_flow
from flows.document import transfer as doc_transfer_flow
from flows.file import upload_download as file_upload_download_flow
from flows.inventory import list_and_alert as inv_list_alert_flow
from flows.inventory import non_std_convert as inv_non_std_flow
from flows.product import barcode_lookup as product_barcode_flow
from flows.product import product_crud as product_crud_flow
from flows.report import inventory_overview as report_inventory_flow
from flows.report import sales_stats as report_sales_flow
from flows.user import user_crud as user_crud_flow
from flows.warehouse import switch_warehouse as wh_switch_flow
from flows.warehouse import warehouse_crud as wh_crud_flow


_log = get_logger("cli")


def _build_client() -> APIClient:
    """创建一个配置好的 APIClient，默认带上 default_warehouse_id。"""
    client = APIClient(base_url=CONFIG.base_url)
    if CONFIG.default_warehouse_id is not None:
        client.set_warehouse(CONFIG.default_warehouse_id)
        SESSION.warehouse_id = CONFIG.default_warehouse_id
    return client


def _run_or_exit(label: str, fn) -> None:
    """统一异常处理：把业务异常映射成 CLI 退出码。"""
    _log.info("=== 开始执行流程：%s ===", label)
    try:
        fn()
    except NotImplementedError as e:
        _log.warning("%s 未实现：%s", label, e)
        sys.exit(2)
    except APIError as e:
        _log.error("%s 失败：APIError code=%s message=%s", label, e.code, e.message)
        sys.exit(3)
    except AssertionError as e:
        _log.error("%s 断言失败：%s", label, e)
        sys.exit(4)
    except Exception as e:  # noqa: BLE001
        _log.error("%s 未预期异常：%s\n%s", label, e, traceback.format_exc())
        sys.exit(5)
    _log.info("=== 流程结束：%s OK ===", label)


# ==================== Click 命令树 ====================


@click.group()
def cli() -> None:
    """RIMS 前端验证器。"""


# ---------- 顶层便捷命令 ----------


@cli.command("smoke")
def smoke() -> None:
    """最小冒烟：登录 → /users/me → /warehouses 列表。"""

    def _flow() -> None:
        client = _build_client()
        # 第 1 步：登录（内部会自动调 /users/me 校验 token）
        auth_login_flow.run(client, CONFIG.admin_user, CONFIG.admin_password)
        # 第 2 步：拉仓库列表，验证基本读接口
        _log.info("获取当前用户可见的仓库列表")
        data = client.get("/warehouses", scoped=False, params={"page": 1, "pageSize": 10})
        # 响应是分页结构，打印一下 total 方便肉眼验证
        total = data.get("total") if isinstance(data, dict) else "?"
        _log.info("仓库列表 total=%s", total)

    _run_or_exit("smoke", _flow)


# ---------- auth 子命令组 ----------


@cli.group()
def auth() -> None:
    """鉴权相关流程。"""


@auth.command("login")
@click.option("-u", "--username", default=None, help="用户名，缺省读 .env")
@click.option("-p", "--password", default=None, help="密码，缺省读 .env")
def auth_login_cmd(username: str | None, password: str | None) -> None:
    """执行登录流程并打印当前用户信息。"""

    def _flow() -> None:
        client = _build_client()
        auth_login_flow.run(
            client,
            username or CONFIG.admin_user,
            password or CONFIG.admin_password,
        )

    _run_or_exit("auth.login", _flow)


# ---------- 其它模块子命令组（全部是 TODO 占位） ----------


def _register_todo(group: click.Group, name: str, flow_module, label: str) -> None:
    """把 flow 模块的 run() 注册成一个 click 子命令（登录 → run）。"""

    @group.command(name)
    def _cmd() -> None:  # noqa: D401
        """（TODO）调用对应业务流程。"""

        def _flow() -> None:
            client = _build_client()
            # TODO 流程统一先登录，再调 run()，即使现在 run() 会抛 NotImplemented
            auth_login_flow.run(client, CONFIG.admin_user, CONFIG.admin_password)
            flow_module.run(client)

        _run_or_exit(label, _flow)

    _cmd.__doc__ = f"执行 {label} 流程（若未实现会退出码 2）。"


@cli.group()
def user() -> None:
    """用户/角色/权限流程。"""


_register_todo(user, "crud", user_crud_flow, "user.user_crud")


@cli.group()
def warehouse() -> None:
    """仓库流程。"""


_register_todo(warehouse, "crud", wh_crud_flow, "warehouse.warehouse_crud")
_register_todo(warehouse, "switch", wh_switch_flow, "warehouse.switch_warehouse")


@cli.group()
def product() -> None:
    """商品档案流程。"""


_register_todo(product, "crud", product_crud_flow, "product.product_crud")
_register_todo(product, "barcode", product_barcode_flow, "product.barcode_lookup")


@cli.group()
def inventory() -> None:
    """库存流程。"""


_register_todo(inventory, "list", inv_list_alert_flow, "inventory.list_and_alert")
_register_todo(inventory, "non-std", inv_non_std_flow, "inventory.non_std_convert")


@cli.group()
def document() -> None:
    """业务单据流程。"""


_register_todo(document, "inbound", doc_inbound_flow, "document.inbound")
_register_todo(document, "sale", doc_sale_flow, "document.sale")
_register_todo(document, "return", doc_return_flow, "document.return_flow")
_register_todo(document, "transfer", doc_transfer_flow, "document.transfer")
_register_todo(document, "stocktake", doc_stocktake_flow, "document.stocktake")


@cli.group()
def report() -> None:
    """报表流程。"""


_register_todo(report, "sales", report_sales_flow, "report.sales_stats")
_register_todo(report, "inventory", report_inventory_flow, "report.inventory_overview")


@cli.group()
def file() -> None:
    """文件附件流程。"""


_register_todo(file, "upload", file_upload_download_flow, "file.upload_download")


@cli.group()
def audit() -> None:
    """审计日志流程。"""


_register_todo(audit, "logs", audit_query_logs_flow, "audit.query_logs")


if __name__ == "__main__":
    cli()
