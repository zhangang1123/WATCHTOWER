#!/usr/bin/env python3
"""
SRE Agent - ReAct 推理引擎
Think -> Act -> Observe -> 循环直到定位根因
"""
import json
import re
import time
from typing import Dict, Any, Optional, AsyncGenerator

import httpx


class SREAgent:
    """AIOps 诊断 Agent"""

    LLM_TOOL_NAMES = {"prometheus_query", "k8s_get_events", "k8s_get_logs"}

    def __init__(
        self,
        llm_model: str,
        api_key: str,
        tools: Dict[str, Any],
        *,
        llm_enabled: bool = True,
        llm_provider: str = "deepseek",
        llm_base_url: str = "https://api.deepseek.com",
        llm_timeout: int = 30,
        llm_max_tokens: int = 1200,
        max_iterations: int = 10,
        history_similarity_threshold: float = 0.78,
    ):
        self.llm_model = llm_model
        self.api_key = api_key
        self.tools = tools
        self.llm_enabled = llm_enabled and bool(api_key)
        self.llm_provider = llm_provider
        self.llm_base_url = llm_base_url.rstrip("/")
        self.llm_timeout = llm_timeout
        self.llm_max_tokens = llm_max_tokens
        self.max_iterations = max_iterations
        self.history_similarity_threshold = max(0.0, min(history_similarity_threshold, 1.0))
        mode = f"{llm_provider}/{llm_model}" if self.llm_enabled else "rule fallback"
        print(f"[Agent] reasoning mode: {mode}")

    async def diagnose(self, incident: Dict[str, Any]) -> Dict[str, Any]:
        """单次诊断：ReAct 循环推理"""
        print(f"[Agent] Diagnosing incident: {incident['id']} - {incident['description']}")

        context = {
            "incident": incident,
            "thoughts": [],
            "observations": [],
            "steps": [],
            "display_steps": [],
            "historical_cases": [],
        }

        history_step, similar = await self._retrieve_history(incident)
        context["display_steps"].append(history_step)
        if similar.get("accepted_as_context"):
            context["historical_cases"] = similar.get("matches", [])

        for iteration in range(self.max_iterations):
            # Think: 决定下一步查什么
            thought = await self._think(context)
            context["thoughts"].append(thought)

            trace_step = self._trace_step(thought)
            context["display_steps"].append(trace_step)

            if thought.get("is_final"):
                return self._build_diagnosis(context, incident, thought)

            # Act: 调用工具
            tool_name = thought.get("tool_name", "")
            tool_args = thought.get("tool_args", {})

            started = time.perf_counter()
            if tool_name in self.tools:
                observation = await self.tools[tool_name].run(**tool_args)
            else:
                observation = {"error": f"Tool {tool_name} not found"}
            latency_ms = int((time.perf_counter() - started) * 1000)

            context["observations"].append(observation)
            tool_step = {
                "thought": thought.get("reasoning", ""),
                "action": f"tool:{tool_name}",
                "observation": json.dumps({
                    "trace_type": "tool",
                    "tool_name": tool_name,
                    "input": tool_args,
                    "output": observation,
                }, ensure_ascii=False),
                "latency_ms": latency_ms,
            }
            context["steps"].append(tool_step)
            context["display_steps"].append(tool_step)

            print(f"[Agent] Step {iteration + 1}: {tool_name} -> {str(observation)[:100]}...")

        # 超出迭代次数
        return {
            "root_cause": "无法在限定步骤内确定根因",
            "confidence": 0.0,
            "evidence": [],
            "suggested_fix": "",
            "fix_type": "",
            "auto_fixable": False,
            "escalated": True,
            "summary": "诊断步数超限，建议人工介入",
            "diagnosis_path": context.get("display_steps", context["steps"]),
        }

    async def diagnose_stream(self, incident: Dict[str, Any]) -> AsyncGenerator[Dict[str, Any], None]:
        """流式诊断：实时推送每一步"""
        context = {
            "incident": incident,
            "thoughts": [],
            "observations": [],
            "steps": [],
            "display_steps": [],
            "historical_cases": [],
        }

        history_step, similar = await self._retrieve_history(incident)
        context["display_steps"].append(history_step)
        if similar.get("accepted_as_context"):
            context["historical_cases"] = similar.get("matches", [])
        yield {"is_final": False, **history_step}

        for iteration in range(self.max_iterations):
            thought = await self._think(context)
            context["thoughts"].append(thought)

            trace_step = self._trace_step(thought)
            context["display_steps"].append(trace_step)
            yield {"is_final": False, **trace_step}

            if thought.get("is_final"):
                diagnosis = self._build_diagnosis(context, incident, thought)
                yield {
                    "is_final": True,
                    "thought": thought.get("reasoning", ""),
                    "action": "最终结论",
                    "observation": diagnosis["root_cause"],
                    "result": diagnosis,
                }
                return

            tool_name = thought.get("tool_name", "")
            tool_args = thought.get("tool_args", {})

            started = time.perf_counter()
            if tool_name in self.tools:
                observation = await self.tools[tool_name].run(**tool_args)
            else:
                observation = {"error": f"Tool {tool_name} not found"}
            latency_ms = int((time.perf_counter() - started) * 1000)

            context["observations"].append(observation)
            tool_step = {
                "thought": thought.get("reasoning", ""),
                "action": f"tool:{tool_name}",
                "observation": json.dumps({
                    "trace_type": "tool",
                    "tool_name": tool_name,
                    "input": tool_args,
                    "output": observation,
                }, ensure_ascii=False),
                "latency_ms": latency_ms,
            }
            context["steps"].append(tool_step)
            context["display_steps"].append(tool_step)

            yield {
                "is_final": False,
                **tool_step,
            }

        # 超出迭代次数
        diagnosis = {
            "root_cause": "无法在限定步骤内确定根因",
            "confidence": 0.0,
            "evidence": [],
            "suggested_fix": "",
            "fix_type": "",
            "auto_fixable": False,
            "escalated": True,
            "summary": "诊断步数超限",
            "diagnosis_path": context.get("display_steps", context["steps"]),
        }
        yield {
            "is_final": True,
            "thought": "超出诊断步数限制",
            "action": "升级人工处理",
            "observation": "需要人工介入",
            "result": diagnosis,
        }

    async def _check_history(self, incident: Dict[str, Any]) -> Optional[Dict[str, Any]]:
        """检查历史故障模式"""
        if "similar_incidents" in self.tools:
            query = (
                f"服务：{incident.get('service', '')}\n"
                f"严重级别：{incident.get('severity', '')}\n"
                f"故障描述：{incident.get('description', '')}"
            )
            return await self.tools["similar_incidents"].run(query, top_k=3)
        return {"found": False, "retrieval_mode": "disabled"}

    async def _retrieve_history(self, incident: Dict[str, Any]) -> Any:
        """检索相似案例并生成可展示的工具追踪；历史案例只作参考，不直接授予执行权。"""
        started = time.perf_counter()
        result = await self._check_history(incident) or {"found": False}
        similarity = float(result.get("similarity", 0) or 0)
        result["threshold"] = self.history_similarity_threshold
        result["accepted_as_context"] = bool(result.get("found")) and similarity >= self.history_similarity_threshold
        step = {
            "thought": "先通过 Embedding 向量检索查找相似历史故障，作为本次诊断的参考信息。",
            "action": "tool:similar_incidents",
            "observation": json.dumps(
                {
                    "trace_type": "tool",
                    "tool_name": "similar_incidents",
                    "input": {
                        "service": incident.get("service", ""),
                        "description": incident.get("description", ""),
                        "top_k": 3,
                    },
                    "output": result,
                },
                ensure_ascii=False,
            ),
            "latency_ms": int((time.perf_counter() - started) * 1000),
        }
        return step, result

    async def _think(self, context: Dict[str, Any]) -> Dict[str, Any]:
        """
        Think: LLM 推理下一步该查什么
        简化版：基于规则推理，实际项目应调用 LLM API
        """
        if self.llm_enabled:
            request = self._build_llm_request(context)
            started = time.perf_counter()
            try:
                return await self._think_with_llm(context, request, started)
            except Exception as error:
                print(f"[Agent] LLM request failed; using rule fallback: {type(error).__name__}: {error}")
                fallback = await self._think_with_rules(context)
                fallback["_trace"] = {
                    "trace_type": "llm",
                    "mode": "fallback_after_error",
                    "provider": self.llm_provider,
                    "model": self.llm_model,
                    "endpoint": f"{self.llm_base_url}/chat/completions",
                    "input": request,
                    "output": {"error": f"{type(error).__name__}: {error}", "fallback": fallback.copy()},
                    "latency_ms": int((time.perf_counter() - started) * 1000),
                }
                return fallback
        fallback = await self._think_with_rules(context)
        fallback["_trace"] = {
            "trace_type": "rule",
            "mode": "rule_fallback",
            "provider": self.llm_provider,
            "model": self.llm_model,
            "input": {
                "incident": context["incident"],
                "previous_steps": context["steps"][-5:],
                "reason": "LLM disabled or API key not configured",
            },
            "output": {key: value for key, value in fallback.items() if key != "_trace"},
            "latency_ms": 0,
        }
        return fallback

    def _build_llm_request(self, context: Dict[str, Any]) -> Dict[str, Any]:
        payload_context = {
            "incident": context["incident"],
            "previous_steps": context["steps"][-5:],
            "similar_historical_cases": context.get("historical_cases", []),
            "available_tools": {
                "prometheus_query": {"query": "PromQL string"},
                "k8s_get_events": {"namespace": "service namespace"},
                "k8s_get_logs": {"namespace": "service namespace", "tail": 50},
            },
        }
        system_prompt = (
            "你是一名谨慎的 SRE 故障诊断助手。所有解释、判断和总结必须使用中文。"
            "每次只能选择一个可用工具，证据充分时结束诊断。只返回 JSON，不要输出 Markdown。"
            "固定字段为 reasoning、tool_name、tool_args、is_final。禁止虚构工具。"
            "当 is_final=true 时，必须增加 diagnosis 对象，包含 root_cause、confidence、"
            "suggested_fix、fix_type、auto_fixable、summary。summary 必须是一句明确的中文总结结论。"
            "fix_type 只能是 restart_pod、scale、cleanup_disk、config_change、traffic_switch 或空字符串。"
        )
        return {
            "model": self.llm_model,
            "messages": [
                {"role": "system", "content": system_prompt},
                {"role": "user", "content": json.dumps(payload_context, ensure_ascii=False)},
            ],
            "response_format": {"type": "json_object"},
            "max_tokens": self.llm_max_tokens,
            "temperature": 0.1,
        }

    async def _think_with_llm(
        self,
        context: Dict[str, Any],
        request: Dict[str, Any],
        started: float,
    ) -> Dict[str, Any]:
        """Ask an OpenAI-compatible provider (DeepSeek by default) for the next step."""
        headers = {"Authorization": f"Bearer {self.api_key}", "Content-Type": "application/json"}
        async with httpx.AsyncClient(timeout=self.llm_timeout) as client:
            response = await client.post(f"{self.llm_base_url}/chat/completions", headers=headers, json=request)
            response.raise_for_status()
        content = response.json()["choices"][0]["message"]["content"]
        content = re.sub(r"^```(?:json)?\s*|\s*```$", "", content.strip(), flags=re.IGNORECASE)
        result = json.loads(content)
        tool_name = result.get("tool_name", "")
        is_final = bool(result.get("is_final", False))
        if not is_final and (tool_name not in self.LLM_TOOL_NAMES or tool_name not in self.tools):
            raise ValueError(f"LLM selected unsupported tool: {tool_name}")
        tool_args = self._sanitize_tool_args(tool_name, result.get("tool_args", {}), context["incident"])
        return {
            "reasoning": str(result.get("reasoning", "")),
            "tool_name": "" if is_final else tool_name,
            "tool_args": tool_args if not is_final else {},
            "is_final": is_final,
            "diagnosis": result.get("diagnosis") if isinstance(result.get("diagnosis"), dict) else None,
            "_trace": {
                "trace_type": "llm",
                "mode": "live",
                "provider": self.llm_provider,
                "model": self.llm_model,
                "endpoint": f"{self.llm_base_url}/chat/completions",
                "input": request,
                "output": {
                    "raw_content": content,
                    "parsed": result,
                    "usage": response.json().get("usage", {}),
                },
                "latency_ms": int((time.perf_counter() - started) * 1000),
            },
        }

    @staticmethod
    def _trace_step(thought: Dict[str, Any]) -> Dict[str, Any]:
        trace = thought.get("_trace", {})
        is_llm = trace.get("trace_type") == "llm"
        return {
            "thought": thought.get("reasoning", ""),
            "action": "llm:decision" if is_llm else "rule:decision",
            "observation": json.dumps(trace, ensure_ascii=False),
            "latency_ms": int(trace.get("latency_ms", 0)),
        }

    @staticmethod
    def _sanitize_tool_args(tool_name: str, raw_args: Any, incident: Dict[str, Any]) -> Dict[str, Any]:
        args = raw_args if isinstance(raw_args, dict) else {}
        service = str(incident.get("service", "default"))
        if tool_name == "prometheus_query":
            query = args.get("query")
            return {"query": str(query) if query else f"up{{service='{service}'}}"}
        if tool_name == "k8s_get_events":
            return {"namespace": str(args.get("namespace") or service)}
        if tool_name == "k8s_get_logs":
            try:
                tail = max(1, min(int(args.get("tail", 50)), 500))
            except (TypeError, ValueError):
                tail = 50
            return {"namespace": str(args.get("namespace") or service), "tail": tail}
        return {}

    async def _think_with_rules(self, context: Dict[str, Any]) -> Dict[str, Any]:
        """Deterministic fallback used when no API key is configured or the API fails."""
        incident = context["incident"]
        steps = context["steps"]

        # 基于场景的规则推理（简化版）
        description = incident.get("description", "").lower()

        if len(steps) == 0:
            # 第一步：总是先查指标
            return {
                "reasoning": f"告警: {incident['description']}，先查询服务指标确认问题",
                "tool_name": "prometheus_query",
                "tool_args": {"query": f"up{{service='{incident['service']}'}}"},
                "is_final": False,
            }

        last_observation = steps[-1].get("observation", "")

        if len(steps) == 1:
            # 第二步：查 K8s 事件
            return {
                "reasoning": "服务状态已确认，查询 K8s 事件寻找变更线索",
                "tool_name": "k8s_get_events",
                "tool_args": {"namespace": incident["service"]},
                "is_final": False,
            }

        if len(steps) == 2:
            # 第三步：查日志
            return {
                "reasoning": "K8s 事件已获取，查看 Pod 日志寻找错误信息",
                "tool_name": "k8s_get_logs",
                "tool_args": {"namespace": incident["service"], "tail": 50},
                "is_final": False,
            }

        # 第四步：综合判断，给出结论
        return await self._generate_conclusion(context)

    async def _generate_conclusion(self, context: Dict[str, Any]) -> Dict[str, Any]:
        """生成最终诊断结论"""
        incident = context["incident"]
        steps = context["steps"]

        # 基于告警描述和观察结果推断根因（简化版）
        description = incident.get("description", "").lower()

        if "oom" in description or "memory" in description or "内存" in description:
            return {
                "reasoning": "综合指标、事件和日志，确认是内存不足导致的 OOMKilled",
                "is_final": True,
            }
        elif "cpu" in description:
            return {
                "reasoning": "CPU 使用率持续高位，确认是计算资源不足",
                "is_final": True,
            }
        elif "latency" in description or "slow" in description or "延迟" in description:
            return {
                "reasoning": "P99 延迟飙升，日志显示数据库连接池耗尽，根因是数据库连接问题",
                "is_final": True,
            }
        elif "disk" in description or "磁盘" in description:
            return {
                "reasoning": "磁盘使用率超过阈值，需要清理日志文件",
                "is_final": True,
            }
        elif "timeout" in description or "conn" in description or "连接" in description or "超时" in description:
            return {
                "reasoning": "连接超时，检查下游依赖服务状态",
                "is_final": True,
            }
        elif "crash" in description or "崩溃" in description or "重启" in description:
            return {
                "reasoning": "Pod 反复崩溃，日志显示 panic，代码存在 bug",
                "is_final": True,
            }

        return {
            "reasoning": "无法从现有信息确定根因，需要人工介入",
            "is_final": True,
        }

    def _build_diagnosis(
        self,
        context: Dict[str, Any],
        incident: Dict[str, Any],
        final_thought: Optional[Dict[str, Any]] = None,
    ) -> Dict[str, Any]:
        """构建诊断结果"""
        description = incident.get("description", "").lower()

        # 基于告警类型匹配预设诊断（简化版）
        diagnosis_map = {
            "oom": {
                "root_cause": "Pod 内存使用超过限制，触发 OOMKilled",
                "confidence": 0.92,
                "evidence": [
                    {"source": "k8s", "description": "K8s 事件显示 OOMKilling", "raw_data": "Memory cgroup out of memory"},
                    {"source": "logs", "description": "内核日志确认 OOM", "raw_data": "Killed process ... anon-rss:1048576kB"},
                ],
                "suggested_fix": "1. 临时: kubectl set resources deployment/{service} --limits=memory=2Gi\n2. 永久: 优化内存使用或增加节点内存",
                "fix_type": "config_change",
                "auto_fixable": False,
                "summary": "总结结论：内存限制不足导致服务退出；修改资源配置风险较高，已转人工确认。",
            },
            "cpu": {
                "root_cause": "CPU 使用率持续超过 90%，计算资源不足",
                "confidence": 0.94,
                "evidence": [
                    {"source": "prometheus", "description": "CPU 使用率 95%", "raw_data": "container_cpu_usage_seconds_total = 0.95"},
                    {"source": "k8s", "description": "CPU throttling detected", "raw_data": "..."},
                ],
                "suggested_fix": "1. 临时: kubectl scale deployment/{service} --replicas=5\n2. 永久: 优化代码性能或增加节点 CPU",
                "fix_type": "scale",
                "auto_fixable": True,
                "summary": "总结结论：CPU 资源不足，模型建议自动扩容，风险校验通过后可自动执行。",
            },
            "latency": {
                "root_cause": "数据库连接池耗尽导致查询延迟飙升",
                "confidence": 0.85,
                "evidence": [
                    {"source": "prometheus", "description": "P99 延迟 2.5s", "raw_data": "..."},
                    {"source": "logs", "description": "连接池耗尽", "raw_data": "database connection pool exhausted"},
                ],
                "suggested_fix": "1. 增加数据库连接池大小\n2. 检查慢查询并优化索引\n3. 考虑读写分离",
                "fix_type": "config_change",
                "auto_fixable": False,
                "summary": "总结结论：数据库连接池耗尽导致请求变慢，配置变更需由人工审核。",
            },
            "disk": {
                "root_cause": "日志文件积累导致磁盘使用率超过 85%",
                "confidence": 0.90,
                "evidence": [
                    {"source": "prometheus", "description": "磁盘使用率 92%", "raw_data": "..."},
                ],
                "suggested_fix": "1. 清理 7 天前的日志\n2. 配置日志轮转\n3. 扩容磁盘",
                "fix_type": "cleanup_disk",
                "auto_fixable": False,
                "summary": "总结结论：日志积累导致磁盘空间不足，清理操作需要人工确认。",
            },
            "timeout": {
                "root_cause": "Redis 连接数达到上限，新连接被拒绝",
                "confidence": 0.87,
                "evidence": [
                    {"source": "logs", "description": "Redis 连接超时", "raw_data": "connection timeout to redis:6379"},
                ],
                "suggested_fix": "1. 临时: redis-cli CONFIG SET maxclients 2048\n2. 永久: 优化连接池配置",
                "fix_type": "config_change",
                "auto_fixable": False,
                "summary": "总结结论：Redis 连接数达到上限，配置变更风险较高，已转人工处理。",
            },
            "crash": {
                "root_cause": "代码 panic 导致 Pod 反复崩溃",
                "confidence": 0.96,
                "evidence": [
                    {"source": "logs", "description": "panic 日志", "raw_data": "panic: runtime error: index out of range"},
                    {"source": "k8s", "description": "CrashLoopBackOff", "raw_data": "Back-off restarting failed container"},
                ],
                "suggested_fix": "1. 回滚到上一个稳定版本\n2. 修复代码中的越界访问\n3. 增加边界检查",
                "fix_type": "restart_pod",
                "auto_fixable": True,
                "summary": "总结结论：Pod 因程序异常反复崩溃，模型建议自动重启 Pod；该动作属于低风险操作。",
            },
        }

        # 匹配根因
        matched = None
        scenario_patterns = [
            ("oom", ("oom", "memory", "内存")),
            ("cpu", ("cpu",)),
            ("latency", ("latency", "slow", "延迟")),
            ("disk", ("disk", "磁盘")),
            ("timeout", ("timeout", "conn", "连接", "超时")),
            ("crash", ("crash", "崩溃", "重启")),
        ]
        matched_key = ""
        for key, patterns in scenario_patterns:
            if any(pattern in description for pattern in patterns):
                matched = diagnosis_map[key]
                matched_key = key
                break

        if not matched:
            matched = {
                "root_cause": "无法确定根因",
                "confidence": 0.3,
                "evidence": [],
                "suggested_fix": "需要人工排查",
                "fix_type": "",
                "auto_fixable": False,
                "summary": "总结结论：现有证据不足以确定根因，系统已转交人工继续排查。",
            }

        model_diagnosis = (final_thought or {}).get("diagnosis")
        if isinstance(model_diagnosis, dict):
            model_root_cause = str(model_diagnosis.get("root_cause", "")).strip()
            model_summary = str(model_diagnosis.get("summary", "")).strip()
            if model_root_cause and re.search(r"[\u4e00-\u9fff]", model_root_cause):
                matched["root_cause"] = model_root_cause
            if model_summary and re.search(r"[\u4e00-\u9fff]", model_summary):
                matched["summary"] = model_summary if model_summary.startswith("总结结论") else f"总结结论：{model_summary}"

            # 模型可以提出动作，但自动执行权限仍由本地场景策略与 Go 风险引擎控制。
            proposed_fix = str(model_diagnosis.get("suggested_fix", "")).strip()
            if proposed_fix and re.search(r"[\u4e00-\u9fff]", proposed_fix):
                matched["suggested_fix"] = proposed_fix
            if matched_key not in {"cpu", "crash"}:
                allowed_fix_types = {"restart_pod", "scale", "cleanup_disk", "config_change", "traffic_switch", ""}
                proposed_type = str(model_diagnosis.get("fix_type", ""))
                if proposed_type in allowed_fix_types:
                    matched["fix_type"] = proposed_type
                matched["auto_fixable"] = False

        # 填充服务名
        service = incident.get("service", "service")
        matched["suggested_fix"] = matched["suggested_fix"].replace("{service}", service)
        historical_cases = context.get("historical_cases", [])
        if historical_cases:
            best = historical_cases[0]
            matched["evidence"] = list(matched.get("evidence", []))
            matched["evidence"].append({
                "source": "history_embedding",
                "description": (
                    f"Embedding 检索命中历史案例 {best.get('case_id', '')}，"
                    f"余弦相似度 {float(best.get('similarity', 0)):.2f}；仅作为诊断参考。"
                ),
                "raw_data": json.dumps(best, ensure_ascii=False),
            })
        matched["diagnosis_path"] = context.get("display_steps", context["steps"])
        if not matched.get("summary"):
            matched["summary"] = f"总结结论：{matched['root_cause']}"
        matched["escalated"] = not matched["auto_fixable"]

        return matched
