#!/usr/bin/env python3
"""K8s 查询工具"""
import httpx
from typing import Any, Dict, List
from .base import BaseTool


class K8sEventsTool(BaseTool):
    """K8s 事件查询工具"""

    name = "k8s_get_events"
    description = "查询 K8s 集群事件，寻找变更线索"

    def __init__(self, base_url: str):
        self.base_url = base_url
        self.client = httpx.AsyncClient(timeout=10.0)

    async def run(self, namespace: str, resource_type: str = "") -> Dict[str, Any]:
        try:
            resp = await self.client.get(
                f"{self.base_url}/mock/k8s/events",
                params={"namespace": namespace, "resource_type": resource_type},
            )
            resp.raise_for_status()
            events = resp.json()
            return {
                "namespace": namespace,
                "events": events,
                "count": len(events),
                "status": "success",
            }
        except Exception as e:
            return {"status": "error", "error": str(e)}


class K8sLogsTool(BaseTool):
    """K8s Pod 日志查询工具"""

    name = "k8s_get_logs"
    description = "查询 Pod 日志，寻找错误信息"

    def __init__(self, base_url: str):
        self.base_url = base_url
        self.client = httpx.AsyncClient(timeout=10.0)

    async def run(self, namespace: str, pod: str = "", tail: int = 100) -> Dict[str, Any]:
        try:
            resp = await self.client.get(
                f"{self.base_url}/mock/k8s/logs",
                params={"namespace": namespace, "pod": pod, "tail": tail},
            )
            resp.raise_for_status()
            logs = resp.json()
            return {
                "namespace": namespace,
                "pod": pod,
                "logs": logs,
                "status": "success",
            }
        except Exception as e:
            return {"status": "error", "error": str(e)}
