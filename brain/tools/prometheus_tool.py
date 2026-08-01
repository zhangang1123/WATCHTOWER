#!/usr/bin/env python3
"""Prometheus 查询工具"""
import httpx
from typing import Any, Dict
from .base import BaseTool


class PrometheusTool(BaseTool):
    """PromQL 查询工具"""

    name = "prometheus_query"
    description = "执行 PromQL 查询，获取服务指标数据"

    def __init__(self, base_url: str):
        self.base_url = base_url
        self.client = httpx.AsyncClient(timeout=10.0)

    async def run(self, query: str, time_range: str = "15m") -> Dict[str, Any]:
        """执行 PromQL 查询"""
        try:
            resp = await self.client.get(
                f"{self.base_url}/api/v1/query",
                params={"query": query},
            )
            resp.raise_for_status()
            data = resp.json()

            if data.get("status") == "success":
                result = data.get("data", {})
                return {
                    "query": query,
                    "result_type": result.get("resultType", "unknown"),
                    "data": result.get("result", []),
                    "status": "success",
                }
            else:
                return {
                    "query": query,
                    "status": "error",
                    "error": data.get("error", "unknown error"),
                }
        except Exception as e:
            return {
                "query": query,
                "status": "error",
                "error": str(e),
            }
