#!/usr/bin/env python3
"""内存向量存储（简化版，用于故障模式检索）"""
from typing import Any, Dict, List
import asyncio


class MemoryVectorStore:
    """内存向量库：简化版，用关键词匹配替代真实向量检索"""

    def __init__(self):
        self.patterns: List[Dict[str, Any]] = []
        self._init_sample_data()

    def _init_sample_data(self):
        """初始化示例故障模式"""
        self.patterns = [
            {
                "id": "pattern-1",
                "keywords": ["oom", "memory", "killed"],
                "service": "payment",
                "root_cause": "Pod 内存使用超过限制，触发 OOMKilled",
                "fix_plan": "增加内存限制或优化内存使用",
                "diagnosis_path": [],
            },
            {
                "id": "pattern-2",
                "keywords": ["cpu", "throttling", "high"],
                "service": "user",
                "root_cause": "CPU 使用率持续超过 90%",
                "fix_plan": "扩容副本数或优化代码",
                "diagnosis_path": [],
            },
            {
                "id": "pattern-3",
                "keywords": ["latency", "slow", "timeout"],
                "service": "order",
                "root_cause": "数据库连接池耗尽",
                "fix_plan": "增加连接池大小或优化查询",
                "diagnosis_path": [],
            },
            {
                "id": "pattern-4",
                "keywords": ["disk", "full", "pressure"],
                "service": "gateway",
                "root_cause": "日志文件积累导致磁盘满",
                "fix_plan": "清理日志并配置轮转",
                "diagnosis_path": [],
            },
            {
                "id": "pattern-5",
                "keywords": ["crash", "panic", "loop"],
                "service": "payment",
                "root_cause": "代码 panic 导致反复崩溃",
                "fix_plan": "回滚版本并修复代码",
                "diagnosis_path": [],
            },
        ]

    async def insert(self, pattern: Dict[str, Any]):
        """插入故障模式"""
        self.patterns.append(pattern)

    async def search(self, query: str, top_k: int = 3) -> List[Dict[str, Any]]:
        """搜索相似故障模式（简化版：关键词匹配）"""
        query_lower = query.lower()
        query_words = set(query_lower.split())

        scored = []
        for pattern in self.patterns:
            score = 0.0
            for kw in pattern["keywords"]:
                if kw in query_lower:
                    score += 0.3
            # 额外加分：如果服务名匹配
            if pattern.get("service", "") in query_lower:
                score += 0.2

            if score > 0:
                scored.append((score, pattern))

        # 按相似度排序
        scored.sort(key=lambda x: x[0], reverse=True)

        results = []
        for score, pattern in scored[:top_k]:
            result = dict(pattern)
            result["similarity"] = min(score, 1.0)
            results.append(result)

        return results

    async def learn(self, incident: Dict[str, Any], diagnosis: Dict[str, Any]):
        """学习新故障模式"""
        description = incident.get("description", "")
        keywords = self._extract_keywords(description)

        pattern = {
            "id": f"pattern-{len(self.patterns) + 1}",
            "keywords": keywords,
            "service": incident.get("service", ""),
            "root_cause": diagnosis.get("root_cause", ""),
            "fix_plan": diagnosis.get("suggested_fix", ""),
            "diagnosis_path": diagnosis.get("diagnosis_path", []),
        }
        await self.insert(pattern)

    def _extract_keywords(self, text: str) -> List[str]:
        """提取关键词（简化版）"""
        keywords = []
        text_lower = text.lower()
        keyword_map = {
            "oom": ["oom", "memory", "killed"],
            "cpu": ["cpu", "throttling"],
            "latency": ["latency", "slow", "timeout"],
            "disk": ["disk", "full", "pressure"],
            "crash": ["crash", "panic", "loop"],
            "conn": ["connection", "timeout", "refused"],
        }
        for _, words in keyword_map.items():
            for w in words:
                if w in text_lower:
                    keywords.extend(words)
                    break
        return list(set(keywords))[:5]
