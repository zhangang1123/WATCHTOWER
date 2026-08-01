#!/usr/bin/env python3
"""工具基类"""
from abc import ABC, abstractmethod
from typing import Any, Dict


class BaseTool(ABC):
    """Agent 工具基类"""

    name: str = ""
    description: str = ""

    @abstractmethod
    async def run(self, **kwargs) -> Dict[str, Any]:
        """执行工具"""
        pass
