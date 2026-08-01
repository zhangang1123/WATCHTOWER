# Watchtower-Lite

> 个人版 AIOps 智能运维 Agent 平台 —— Go + Python 混合架构，事件驱动，Agent 自主诊断

## 项目简介

Watchtower-Lite 是一个**精简版 AIOps 平台**，基于企业级 [Watchtower](./../Watchtower-Lite-个人项目设计方案.md) 方案设计，面向个人开发和面试展示。

**核心特性：**
- Go 侧：高并发告警接入、事件状态机、自动修复引擎、WebSocket 实时推送
- Python 侧：ReAct 推理 Agent、工具调用、根因诊断
- 模拟基础设施：无需真实 K8s/Prometheus，内置 Mock 服务即可完整运行
- 实时 Web Console：Vue3 前端，实时展示事件流和诊断过程
- SQLite 事件档案：持久化事件快照、状态时间轴、模型步骤与修复记录
- 工程演练：动态故障状态、修复验证、失败回滚、诊断回放与告警风暴压测

## 架构图

```
[Mock Alert Generator] → [Go Gateway] → [Priority Queue] → [Go Incident Manager]
                                                                  │
                                    ┌─────────────────────────────┼─────────────────────────────┐
                                    │ gRPC                        │ gRPC                        │ WS
                                    ▼                             ▼                             ▼
                              [Python Agent]              [Go AutoFix Engine]            [Web Console]
                              (ReAct 诊断)                (分级风险控制)                  (实时展示)
                                    │                             │
                                    └──────────┬──────────────────┘
                                               │
                                        [SQLite + 内存向量库]
```

## 快速开始

### 方式一：Docker Compose（推荐）

```bash
# 1. 首次克隆后创建私密配置，再填写 DeepSeek 密钥
cp .env.example .env
# LLM_API_KEY=sk-你的DeepSeek密钥

# 2. 一键启动
docker-compose -f deploy/docker-compose.yml up --build

# 3. 访问 Web Console
open http://localhost:8080

# 4. 触发模拟告警
curl -X POST http://localhost:8080/api/v1/mock/trigger \
  -H "Content-Type: application/json" \
  -d '{"scenario": "oom_kill", "service": "payment"}'
```

### 方式二：本地开发

```bash
# Git Bash / WSL
./scripts/start.sh
```

所有端口、内部地址和 LLM 参数统一从项目根目录 `.env` 读取；系统环境变量优先级更高，可用于容器或 CI 覆盖。`.env` 已被 Git 忽略，安全模板见 `.env.example`。未填写 `LLM_API_KEY` 时，Brain 会自动使用规则推理，其他功能仍可演示。

## 支持的故障场景

| 场景 | 触发方式 | Agent 行为 |
|------|---------|-----------|
| OOMKilled | `scenario: oom_kill` | 诊断内存不足 → 建议增加内存限制 |
| CPU 飙升 | `scenario: high_cpu` | 模型诊断 CPU 瓶颈 → 自动模拟扩容并验证 |
| 延迟飙升 | `scenario: high_latency` | 诊断数据库连接池耗尽 → 建议优化 |
| 磁盘满 | `scenario: disk_full` | 诊断日志积累 → 建议清理 |
| 连接超时 | `scenario: conn_timeout` | 诊断 Redis 连接池满 → 建议调参 |
| 反复崩溃 | `scenario: crash_loop` | 模型诊断崩溃循环 → 自动模拟重启并验证 |

网页默认选择“反复崩溃”场景，便于直接观察一次完整的模型诊断、自动修复、观察验证和中文总结结论。高风险动作仍会转人工，这是安全策略的预期行为。

## API 接口

### 告警接入

```bash
# Prometheus Webhook
POST /webhook/prometheus

# 自定义告警
POST /webhook/custom

# 手动触发模拟告警
POST /api/v1/mock/trigger
Body: {"scenario": "oom_kill", "service": "payment"}

# 生成告警风暴并返回去重与吞吐结果
POST /api/v1/mock/storm
Body: {"count": 1000, "duplicate_ratio": 0.8, "service": "storm-demo"}
```

### 事件查询

```bash
# 获取事件列表
GET /api/v1/incidents

# 获取事件详情
GET /api/v1/incidents/:id

# 获取持久化时间轴与诊断回放帧
GET /api/v1/incidents/:id/timeline
GET /api/v1/incidents/:id/replay

# WebSocket 实时事件流
WS /ws
```

## 项目结构

```
watchtower-lite/
├── cmd/
│   ├── gateway/          # Go 主服务入口
│   └── mock-infra/       # Mock 基础设施入口
├── internal/
│   ├── gateway/          # 告警接入网关（去重/聚合/队列）
│   ├── incident/         # 事件管理（状态机）
│   ├── autofix/          # 自动修复引擎
│   ├── agent/            # gRPC 客户端
│   ├── mock/             # 模拟 Prometheus/K8s
│   ├── models/           # 数据模型
│   ├── store/            # SQLite 存储
│   └── ws/               # WebSocket 广播
├── proto/                # gRPC 协议定义
├── brain/                # Python Agent
│   ├── server.py         # gRPC Server
│   ├── agent.py          # ReAct 推理引擎
│   ├── tools/            # 工具集
│   └── memory/           # 故障模式存储
├── deploy/               # Docker Compose 部署
├── Makefile
├── go.mod
└── requirements.txt
```

## 面试亮点

**Go 侧：**
- 高并发告警处理：优先级队列 + goroutine 池
- 严格的事件状态机：防止非法状态转换
- gRPC 流式通信：诊断过程实时回传
- 自动修复分级：低风险自动执行、高风险人工确认

**Python 侧：**
- ReAct 推理引擎：Think → Act → Observe 循环
- 工具调用架构：Agent 自主决定查什么数据
- 故障模式学习：相似告警直接匹配历史方案

**架构：**
- Go+Python 混合：基础设施 + 智能分离
- 事件驱动：无状态服务可水平扩展
- 模拟基础设施层：无需真实 K8s 即可运行

## 开发路线图

- [x] 项目骨架 + 核心模型
- [x] Go 告警网关（去重/聚合/队列）
- [x] SQLite 持久化、历史事件与时间轴
- [x] 动态 Mock 状态机、修复验证和失败回滚
- [x] 告警风暴压测与诊断回放
- [x] GitHub Actions 自动测试与构建
- [x] Go 事件状态机
- [x] Python ReAct Agent
- [x] gRPC 通信协议
- [x] Mock 基础设施层
- [x] Web Console 前端（含明暗主题）
- [ ] SQLite 事件存储
- [ ] 故障模式向量检索
- [ ] 更多故障场景
- [x] Docker Compose 基础部署

## License

MIT
