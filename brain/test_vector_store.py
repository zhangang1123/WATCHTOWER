from __future__ import annotations

import json
import unittest
from typing import List, Sequence

from agent import SREAgent
from memory.vector_store import MemoryVectorStore
from tools.memory_tool import SimilarIncidentTool


class FakeEmbeddingProvider:
    model = "fake-embedding"

    async def embed(self, texts: Sequence[str]) -> List[List[float]]:
        vectors = []
        for text in texts:
            lowered = text.lower()
            if "崩溃" in text or "panic" in lowered or "crashloop" in lowered:
                vectors.append([1.0, 0.0, 0.0])
            elif "cpu" in lowered or "计算资源" in text:
                vectors.append([0.0, 1.0, 0.0])
            else:
                vectors.append([0.0, 0.0, 1.0])
        return vectors


class BrokenEmbeddingProvider:
    model = "broken-embedding"

    async def embed(self, texts: Sequence[str]) -> List[List[float]]:
        raise RuntimeError("provider unavailable")


class VectorStoreTests(unittest.IsolatedAsyncioTestCase):
    async def test_embedding_search_uses_cosine_similarity(self):
        store = MemoryVectorStore(FakeEmbeddingProvider())
        results = await store.search("payment 的 Pod 不断崩溃，需要排查", top_k=3)

        self.assertGreaterEqual(len(results), 1)
        self.assertEqual(results[0]["id"], "pattern-5")
        self.assertEqual(results[0]["retrieval_mode"], "embedding")
        self.assertEqual(results[0]["embedding_model"], "fake-embedding")
        self.assertAlmostEqual(results[0]["similarity"], 1.0)

    async def test_embedding_failure_falls_back_to_keywords(self):
        store = MemoryVectorStore(BrokenEmbeddingProvider(), keyword_fallback=True)
        results = await store.search("CPU 使用率持续过高", top_k=1)

        self.assertEqual(results[0]["id"], "pattern-2")
        self.assertEqual(results[0]["retrieval_mode"], "keyword_fallback")
        self.assertEqual(results[0]["fallback_reason"], "RuntimeError")

    async def test_stream_history_trace_is_visible_but_not_final(self):
        store = MemoryVectorStore(FakeEmbeddingProvider())
        agent = SREAgent(
            llm_model="test",
            api_key="",
            tools={"similar_incidents": SimilarIncidentTool(store)},
            llm_enabled=False,
            history_similarity_threshold=0.78,
        )
        step, result = await agent._retrieve_history(
            {
                "id": "incident-1",
                "service": "payment",
                "severity": "P0",
                "description": "Pod 反复崩溃并持续重启",
            }
        )

        trace = json.loads(step["observation"])
        self.assertEqual(step["action"], "tool:similar_incidents")
        self.assertEqual(trace["tool_name"], "similar_incidents")
        self.assertTrue(result["accepted_as_context"])
        self.assertEqual(result["retrieval_mode"], "embedding")


if __name__ == "__main__":
    unittest.main()
