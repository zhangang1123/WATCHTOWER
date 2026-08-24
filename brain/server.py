#!/usr/bin/env python3
"""
Watchtower Brain - Python gRPC Server for AIOps Diagnosis Agent
"""
import os
import sys
import asyncio
import grpc
from concurrent import futures

# Add parent to path for imports
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from agent import SREAgent
from tools.prometheus_tool import PrometheusTool
from tools.k8s_tool import K8sEventsTool, K8sLogsTool
from tools.memory_tool import SimilarIncidentTool
from memory.vector_store import MemoryVectorStore, OpenAIEmbeddingProvider

# Import generated proto
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..'))
import proto.diagnosis_pb2 as diagnosis_pb2
import proto.diagnosis_pb2_grpc as diagnosis_pb2_grpc
from config import get_bool, get_float, get_int, load_config

load_config()


class DiagnosisServicer(diagnosis_pb2_grpc.DiagnosisServiceServicer):
    """诊断服务实现"""

    def __init__(self):
        # 初始化 Agent
        llm_model = os.getenv("LLM_MODEL", "deepseek-v4-flash")
        llm_api_key = os.getenv("LLM_API_KEY") or os.getenv("DEEPSEEK_API_KEY", "")

        # 工具集
        mock_prometheus_url = os.getenv("MOCK_PROMETHEUS_URL", "http://localhost:9090")
        mock_k8s_url = os.getenv("MOCK_K8S_URL", "http://localhost:8081")

        embedding_api_key = os.getenv("EMBEDDING_API_KEY") or os.getenv("DASHSCOPE_API_KEY", "")
        embedding_base_url = os.getenv("EMBEDDING_BASE_URL", "")
        embedding_enabled = (
            get_bool("EMBEDDING_ENABLED", True)
            and bool(embedding_api_key)
            and bool(embedding_base_url)
        )
        embedding_provider = None
        if embedding_enabled:
            embedding_provider = OpenAIEmbeddingProvider(
                api_key=embedding_api_key,
                base_url=embedding_base_url,
                model=os.getenv("EMBEDDING_MODEL", "qwen3.7-text-embedding"),
                timeout=get_int("EMBEDDING_TIMEOUT_SECONDS", 30),
            )

        vector_store = MemoryVectorStore(
            embedding_provider=embedding_provider,
            keyword_fallback=get_bool("EMBEDDING_KEYWORD_FALLBACK", True),
        )
        print(f"[Brain] history retrieval mode: {vector_store.mode}")

        tools = {
            "prometheus_query": PrometheusTool(mock_prometheus_url),
            "k8s_get_events": K8sEventsTool(mock_k8s_url),
            "k8s_get_logs": K8sLogsTool(mock_k8s_url),
            "similar_incidents": SimilarIncidentTool(vector_store),
        }

        self.agent = SREAgent(
            llm_model=llm_model,
            api_key=llm_api_key,
            tools=tools,
            llm_enabled=get_bool("LLM_ENABLED", True),
            llm_provider=os.getenv("LLM_PROVIDER", "deepseek"),
            llm_base_url=os.getenv("LLM_BASE_URL", "https://api.deepseek.com"),
            llm_timeout=get_int("LLM_TIMEOUT_SECONDS", 30),
            llm_max_tokens=get_int("LLM_MAX_TOKENS", 1200),
            max_iterations=get_int("AGENT_MAX_ITERATIONS", 10),
            history_similarity_threshold=get_float("EMBEDDING_SIMILARITY_THRESHOLD", 0.78),
        )

    async def Diagnose(self, request, context):
        """单次诊断请求"""
        print(f"[Brain] Received diagnosis request for incident: {request.incident_id}")

        result = await self.agent.diagnose({
            "id": request.incident_id,
            "service": request.service,
            "description": request.alert_description,
            "severity": request.severity,
        })

        return self._build_response(request.incident_id, result)

    async def DiagnoseStream(self, request, context):
        """流式诊断（实时推送每一步）"""
        print(f"[Brain] Starting stream diagnosis for: {request.incident_id}")

        step_number = 0
        async for step in self.agent.diagnose_stream({
            "id": request.incident_id,
            "service": request.service,
            "description": request.alert_description,
            "severity": request.severity,
        }):
            step_number += 1
            yield diagnosis_pb2.DiagnosisStep(
                step_number=step_number,
                thought=step.get("thought", ""),
                action=step.get("action", ""),
                observation=step.get("observation", ""),
                latency_ms=step.get("latency_ms", 0),
                is_final=step.get("is_final", False),
                final_result=self._build_response(request.incident_id, step["result"]) if step.get("is_final") else None,
            )

    async def HealthCheck(self, request, context):
        """健康检查"""
        return diagnosis_pb2.HealthResponse(status="healthy")

    def _build_response(self, incident_id, result):
        """构建诊断响应"""
        response = diagnosis_pb2.DiagnoseResponse(
            incident_id=incident_id,
            root_cause=result.get("root_cause", "unknown"),
            confidence=result.get("confidence", 0.0),
            suggested_fix=result.get("suggested_fix", ""),
            fix_type=result.get("fix_type", ""),
            auto_fixable=result.get("auto_fixable", False),
            escalated=result.get("escalated", False),
            summary=result.get("summary", ""),
        )

        for ev in result.get("evidence", []):
            response.evidence.append(diagnosis_pb2.Evidence(
                source=ev.get("source", ""),
                description=ev.get("description", ""),
                raw_data=ev.get("raw_data", ""),
            ))

        for i, step in enumerate(result.get("diagnosis_path", [])):
            response.diagnosis_path.append(diagnosis_pb2.DiagnosisStep(
                step_number=i + 1,
                thought=step.get("thought", ""),
                action=step.get("action", ""),
                observation=step.get("observation", ""),
                latency_ms=step.get("latency_ms", 0),
            ))

        return response


async def serve():
    port = os.getenv("BRAIN_GRPC_PORT", "50052")
    server = grpc.aio.server(futures.ThreadPoolExecutor(max_workers=10))
    diagnosis_pb2_grpc.add_DiagnosisServiceServicer_to_server(DiagnosisServicer(), server)
    server.add_insecure_port(f"[::]:{port}")

    print(f"[Brain] gRPC Server starting on port {port}")
    await server.start()
    print(f"[Brain] gRPC Server ready, listening on :{port}")
    await server.wait_for_termination()


if __name__ == "__main__":
    asyncio.run(serve())
