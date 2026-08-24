#!/usr/bin/env python3
"""基于 OpenAI 兼容 Embeddings API 的内存向量故障案例库。"""

from __future__ import annotations

import asyncio
import math
from typing import Any, Dict, List, Optional, Protocol, Sequence

from openai import AsyncOpenAI


class EmbeddingProvider(Protocol):
    """Embedding 提供者协议，便于在测试中注入确定性实现。"""

    model: str

    async def embed(self, texts: Sequence[str]) -> List[List[float]]:
        ...


class OpenAIEmbeddingProvider:
    """调用阿里云百炼等 OpenAI 兼容的 Embeddings 接口。"""

    def __init__(self, api_key: str, base_url: str, model: str, timeout: int = 30):
        self.model = model
        self._client = AsyncOpenAI(
            api_key=api_key,
            base_url=base_url.rstrip("/"),
            timeout=timeout,
        )

    async def embed(self, texts: Sequence[str]) -> List[List[float]]:
        if not texts:
            return []
        response = await self._client.embeddings.create(model=self.model, input=list(texts))
        ordered = sorted(response.data, key=lambda item: item.index)
        return [list(item.embedding) for item in ordered]


class MemoryVectorStore:
    """保存故障案例向量，并用余弦相似度返回 Top-K 历史案例。"""

    def __init__(
        self,
        embedding_provider: Optional[EmbeddingProvider] = None,
        *,
        keyword_fallback: bool = True,
    ):
        self.embedding_provider = embedding_provider
        self.keyword_fallback = keyword_fallback
        self.patterns: List[Dict[str, Any]] = []
        self._vectors: Dict[str, List[float]] = {}
        self._index_ready = False
        self._index_lock = asyncio.Lock()
        self._init_sample_data()

    @property
    def mode(self) -> str:
        return "embedding" if self.embedding_provider else "keyword_fallback"

    def _init_sample_data(self) -> None:
        self.patterns = [
            {
                "id": "pattern-1",
                "description": "支付服务 Pod 因内存耗尽被终止，Kubernetes 事件出现 OOMKilled",
                "keywords": ["oom", "memory", "killed", "内存", "内存耗尽"],
                "service": "payment",
                "root_cause": "Pod 内存使用超过限制，触发 OOMKilled",
                "fix_plan": "增加内存限制或优化内存使用",
                "fix_type": "config_change",
                "diagnosis_path": [],
            },
            {
                "id": "pattern-2",
                "description": "服务 CPU 使用率持续超过 90%，出现 CPU throttling 和请求积压",
                "keywords": ["cpu", "throttling", "high", "高负载", "计算资源"],
                "service": "user",
                "root_cause": "CPU 使用率持续超过 90%",
                "fix_plan": "扩容副本数或优化代码",
                "fix_type": "scale",
                "diagnosis_path": [],
            },
            {
                "id": "pattern-3",
                "description": "订单服务 P99 延迟升高，日志显示数据库连接池耗尽和查询超时",
                "keywords": ["latency", "slow", "timeout", "延迟", "连接池"],
                "service": "order",
                "root_cause": "数据库连接池耗尽",
                "fix_plan": "增加连接池大小或优化查询",
                "fix_type": "config_change",
                "diagnosis_path": [],
            },
            {
                "id": "pattern-4",
                "description": "网关节点磁盘使用率超过阈值，历史日志积累触发磁盘压力",
                "keywords": ["disk", "full", "pressure", "磁盘", "日志积累"],
                "service": "gateway",
                "root_cause": "日志文件积累导致磁盘满",
                "fix_plan": "清理日志并配置轮转",
                "fix_type": "cleanup_disk",
                "diagnosis_path": [],
            },
            {
                "id": "pattern-5",
                "description": "支付服务 Pod 因代码 panic 反复崩溃并进入 CrashLoopBackOff",
                "keywords": ["crash", "panic", "loop", "崩溃", "反复重启"],
                "service": "payment",
                "root_cause": "代码 panic 导致反复崩溃",
                "fix_plan": "回滚版本并修复代码",
                "fix_type": "restart_pod",
                "diagnosis_path": [],
            },
        ]

    @staticmethod
    def _pattern_text(pattern: Dict[str, Any]) -> str:
        return "\n".join(
            str(value)
            for value in (
                pattern.get("description", ""),
                f"服务：{pattern.get('service', '')}",
                f"根因：{pattern.get('root_cause', '')}",
                f"处理方案：{pattern.get('fix_plan', '')}",
                "关键词：" + " ".join(pattern.get("keywords", [])),
            )
            if value
        )

    async def _ensure_index(self) -> None:
        if self._index_ready or not self.embedding_provider:
            return
        async with self._index_lock:
            if self._index_ready:
                return
            texts = [self._pattern_text(pattern) for pattern in self.patterns]
            vectors = await self.embedding_provider.embed(texts)
            if len(vectors) != len(self.patterns):
                raise ValueError("Embedding 服务返回的向量数量与案例数量不一致")
            for pattern, vector in zip(self.patterns, vectors):
                self._vectors[pattern["id"]] = vector
            self._index_ready = True

    async def insert(self, pattern: Dict[str, Any]) -> None:
        pattern_copy = dict(pattern)
        pattern_copy.setdefault("id", f"pattern-{len(self.patterns) + 1}")
        self.patterns.append(pattern_copy)
        if self.embedding_provider:
            vectors = await self.embedding_provider.embed([self._pattern_text(pattern_copy)])
            if not vectors:
                raise ValueError("Embedding 服务未返回向量")
            self._vectors[pattern_copy["id"]] = vectors[0]

    async def search(self, query: str, top_k: int = 3) -> List[Dict[str, Any]]:
        if self.embedding_provider:
            try:
                await self._ensure_index()
                query_vectors = await self.embedding_provider.embed([query])
                if not query_vectors:
                    return []
                return self._vector_search(query_vectors[0], top_k)
            except Exception as error:
                if not self.keyword_fallback:
                    raise
                results = self._keyword_search(query, top_k)
                for result in results:
                    result["fallback_reason"] = type(error).__name__
                return results
        return self._keyword_search(query, top_k)

    def _vector_search(self, query_vector: Sequence[float], top_k: int) -> List[Dict[str, Any]]:
        scored = []
        for pattern in self.patterns:
            vector = self._vectors.get(pattern["id"])
            if vector:
                scored.append((self._cosine_similarity(query_vector, vector), pattern))
        return self._format_results(scored, top_k, "embedding")

    def _keyword_search(self, query: str, top_k: int) -> List[Dict[str, Any]]:
        query_lower = query.lower()
        scored = []
        for pattern in self.patterns:
            matches = sum(1 for keyword in pattern.get("keywords", []) if keyword.lower() in query_lower)
            score = min(matches * 0.3, 0.9)
            if pattern.get("service", "").lower() in query_lower:
                score += 0.1
            if score > 0:
                scored.append((min(score, 1.0), pattern))
        return self._format_results(scored, top_k, "keyword_fallback")

    def _format_results(self, scored: List[Any], top_k: int, retrieval_mode: str) -> List[Dict[str, Any]]:
        scored.sort(key=lambda item: item[0], reverse=True)
        results = []
        for score, pattern in scored[: max(1, top_k)]:
            result = dict(pattern)
            result["similarity"] = round(max(-1.0, min(float(score), 1.0)), 6)
            result["retrieval_mode"] = retrieval_mode
            result["embedding_model"] = getattr(self.embedding_provider, "model", "") if retrieval_mode == "embedding" else ""
            results.append(result)
        return results

    @staticmethod
    def _cosine_similarity(left: Sequence[float], right: Sequence[float]) -> float:
        if len(left) != len(right) or not left:
            return 0.0
        dot = sum(a * b for a, b in zip(left, right))
        left_norm = math.sqrt(sum(value * value for value in left))
        right_norm = math.sqrt(sum(value * value for value in right))
        if left_norm == 0 or right_norm == 0:
            return 0.0
        return dot / (left_norm * right_norm)

    async def learn(self, incident: Dict[str, Any], diagnosis: Dict[str, Any]) -> None:
        pattern = {
            "id": f"pattern-{len(self.patterns) + 1}",
            "description": incident.get("description", ""),
            "keywords": self._extract_keywords(incident.get("description", "")),
            "service": incident.get("service", ""),
            "root_cause": diagnosis.get("root_cause", ""),
            "fix_plan": diagnosis.get("suggested_fix", ""),
            "fix_type": diagnosis.get("fix_type", ""),
            "diagnosis_path": diagnosis.get("diagnosis_path", []),
        }
        await self.insert(pattern)

    @staticmethod
    def _extract_keywords(text: str) -> List[str]:
        words = [word.strip("，。；：,.!?:;()[]{}") for word in text.lower().split()]
        return [word for word in words if len(word) >= 2][:8]
