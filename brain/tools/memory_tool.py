#!/usr/bin/env python3
"""历史故障检索工具"""
from typing import Any, Dict
from .base import BaseTool


class SimilarIncidentTool(BaseTool):
    """相似故障检索工具"""

    name = "similar_incidents"
    description = "在历史故障库中检索相似案例"

    def __init__(self, vector_store):
        self.store = vector_store

    async def run(self, description: str, top_k: int = 3) -> Dict[str, Any]:
        try:
            results = await self.store.search(description, top_k)
            if results and len(results) > 0:
                best = results[0]
                return {
                    "found": True,
                    "similarity": best.get("similarity", 0),
                    "root_cause": best.get("root_cause", ""),
                    "fix_plan": best.get("fix_plan", ""),
                    "previous_diagnosis": best.get("diagnosis_path", []),
                }
            return {"found": False}
        except Exception as e:
            return {"found": False, "error": str(e)}
