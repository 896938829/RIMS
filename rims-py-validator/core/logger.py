"""日志工具模块。

- 控制台 + 按天滚动文件双写
- 统一格式：[时间] [级别] [模块名] 内容
- 所有子命令、APIClient、flows 都应使用 `get_logger(__name__)`
"""

from __future__ import annotations

import logging
import os
import sys
from logging.handlers import TimedRotatingFileHandler
from pathlib import Path

from .config import CONFIG

_LOG_FORMAT = "[%(asctime)s] [%(levelname)s] [%(name)s] %(message)s"
_DATE_FORMAT = "%Y-%m-%d %H:%M:%S"

# 是否已经初始化过根日志器（防止多次安装 handler 导致重复输出）
_initialized = False


def _ensure_log_dir() -> Path:
    """创建日志目录（不存在则建）并返回路径。"""
    log_dir = Path(CONFIG.log_dir)
    log_dir.mkdir(parents=True, exist_ok=True)
    return log_dir


def _install_handlers() -> None:
    """为根 logger 安装一次控制台 + 文件 handler。"""
    global _initialized
    if _initialized:
        return

    log_dir = _ensure_log_dir()
    formatter = logging.Formatter(_LOG_FORMAT, datefmt=_DATE_FORMAT)

    # Windows 默认控制台编码是 GBK，中文日志会乱码。
    # Python 3.7+ 支持 TextIOWrapper.reconfigure，强制成 UTF-8。
    try:
        sys.stdout.reconfigure(encoding="utf-8")  # type: ignore[attr-defined]
    except (AttributeError, OSError):
        # 某些受限环境（如 pytest 捕获）不支持 reconfigure，忽略即可
        pass

    # 控制台 handler：让运行时肉眼可见
    console = logging.StreamHandler(sys.stdout)
    console.setLevel(logging.INFO)
    console.setFormatter(formatter)

    # 文件 handler：按天切分，保留 7 天
    log_file = log_dir / "run.log"
    file_handler = TimedRotatingFileHandler(
        filename=str(log_file),
        when="midnight",
        backupCount=7,
        encoding="utf-8",
    )
    # 切分后缀使得文件名形如 run.log.2026-04-11
    file_handler.suffix = "%Y-%m-%d"
    file_handler.setLevel(logging.DEBUG)
    file_handler.setFormatter(formatter)

    root = logging.getLogger("rims")
    root.setLevel(logging.DEBUG)
    root.addHandler(console)
    root.addHandler(file_handler)
    root.propagate = False

    _initialized = True


def get_logger(name: str) -> logging.Logger:
    """获取业务 logger；所有模块都应使用这个工厂方法。

    传入 `__name__` 即可，会自动挂到 `rims.*` 命名空间下，
    与 `_install_handlers()` 安装的根 logger 共享输出通道。
    """
    _install_handlers()
    # 约定把所有 logger 统一挂到 rims.* 下
    if not name.startswith("rims"):
        name = f"rims.{name}"
    return logging.getLogger(name)
