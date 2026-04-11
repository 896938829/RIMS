"""占位工具：未实现的 TODO 流程统一抛出该错误。

flows/<module>/<flow>.py 里如果还没实现，就调 `not_implemented("xxx流程")`。
"""

from __future__ import annotations

from core.logger import get_logger

_log = get_logger("flow.todo")


def not_implemented(flow_label: str) -> None:
    """记录并抛出未实现错误，让 CLI 以非零退出码结束。"""
    _log.warning("流程【%s】尚未实现，请参考 README.md 中的 TODO 清单", flow_label)
    raise NotImplementedError(f"{flow_label} 尚未实现，见 README TODO")
