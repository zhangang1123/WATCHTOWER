<template>
  <div class="console-shell">
    <aside class="sidebar">
      <div class="brand">
        <div class="brand-mark"><el-icon><Aim /></el-icon></div>
        <div>
          <strong>WATCHTOWER</strong>
          <span>智能运维平台</span>
        </div>
      </div>

      <nav class="side-nav" aria-label="主导航">
        <button :class="['nav-item', currentView === 'console' && 'active']" @click="currentView = 'console'"><el-icon><DataAnalysis /></el-icon><span>任务控制台</span></button>
        <button :class="['nav-item', currentView === 'history' && 'active']" @click="openHistory"><el-icon><Tickets /></el-icon><span>事件档案</span></button>
        <button :class="['nav-item', currentView === 'lab' && 'active']" @click="currentView = 'lab'"><el-icon><Operation /></el-icon><span>演练与压测</span></button>
      </nav>

      <div class="system-stack">
        <div class="eyebrow">系统服务</div>
        <div class="system-node">
          <span :class="['status-light', health.gateway === 'ok' && 'online']"></span>
          <div><strong>告警网关</strong><small>:8080 · 告警入口</small></div>
        </div>
        <div class="system-node">
          <span :class="['status-light', health.brain === 'ok' && 'online']"></span>
          <div><strong>诊断大脑</strong><small>:50052 · 模型推理</small></div>
        </div>
        <div class="system-node">
          <span class="status-light online"></span>
          <div><strong>模拟基础设施</strong><small>:9090 / :8081</small></div>
        </div>
      </div>

      <div class="sidebar-foot">
        <el-icon><Lock /></el-icon>
        <div><strong>模拟运行模式</strong><span>修复动作仅模拟执行</span></div>
      </div>
    </aside>

    <div class="workspace">
      <header class="topbar">
        <div>
          <div class="breadcrumb">运维中心 / {{ viewTitle }}</div>
          <h1>{{ viewTitle }}</h1>
        </div>
        <div class="top-actions">
          <div :class="['live-pill', wsConnected && 'connected']">
            <span></span>{{ wsConnected ? '实时链路已连接' : '实时链路重连中' }}
          </div>
          <button
            class="icon-button theme-toggle"
            :aria-label="theme === 'dark' ? '切换到浅色模式' : '切换到深色模式'"
            :title="theme === 'dark' ? '切换到浅色模式' : '切换到深色模式'"
            :aria-pressed="theme === 'light'"
            @click="toggleTheme"
          >
            <el-icon><Sunny v-if="theme === 'dark'" /><Moon v-else /></el-icon>
          </button>
          <button class="icon-button" aria-label="刷新数据" @click="refreshAll"><el-icon><Refresh /></el-icon></button>
          <el-button class="trigger-button" type="primary" @click="showTrigger = true">
            <el-icon><Lightning /></el-icon> 注入故障
          </el-button>
        </div>
      </header>

      <main v-if="currentView === 'console'">
        <section class="mission-hero">
          <div class="hero-copy">
            <div class="eyebrow accent">智能事件响应</div>
            <h2>从告警噪声，到可验证的根因。</h2>
            <p>Watchtower 将监控告警转化为事件，自动采集指标、Kubernetes 事件与日志，并通过风险策略决定修复或升级。</p>
            <div class="hero-flow">
              <template v-for="(step, index) in capabilityFlow" :key="step.label">
                <div class="flow-chip">
                  <el-icon><component :is="step.icon" /></el-icon>
                  <span>{{ step.label }}<small>{{ step.detail }}</small></span>
                </div>
                <el-icon v-if="index < capabilityFlow.length - 1" class="flow-arrow"><ArrowRight /></el-icon>
              </template>
            </div>
          </div>
          <div class="hero-signal">
            <div class="radar">
              <div class="radar-ring ring-1"></div>
              <div class="radar-ring ring-2"></div>
              <div class="radar-ring ring-3"></div>
              <div class="radar-sweep"></div>
              <div class="radar-core"><span>{{ activeCount }}</span><small>处理中</small></div>
            </div>
            <span>持续扫描事件态势</span>
          </div>
        </section>

        <section class="metric-grid">
          <article class="metric-card">
            <div class="metric-head"><span>事件总量</span><el-icon><Tickets /></el-icon></div>
            <strong>{{ incidents.length }}</strong>
            <small>SQLite 中持久化保存</small>
          </article>
          <article class="metric-card danger">
            <div class="metric-head"><span>高优先级</span><el-icon><Warning /></el-icon></div>
            <strong>{{ criticalCount }}</strong>
            <small>P0 / P1 立即进入诊断</small>
          </article>
          <article class="metric-card violet">
            <div class="metric-head"><span>处理中</span><el-icon><Cpu /></el-icon></div>
            <strong>{{ activeCount }}</strong>
            <small>诊断、修复与观察阶段</small>
          </article>
          <article class="metric-card success">
            <div class="metric-head"><span>自动化闭环</span><el-icon><CircleCheck /></el-icon></div>
            <strong>{{ automationRate }}%</strong>
            <small>{{ resolvedCount }} 个事件已恢复</small>
          </article>
        </section>

        <section class="main-grid">
          <article class="panel incident-panel">
            <div class="panel-title">
              <div><span class="eyebrow">事件队列</span><h3>实时事件队列</h3></div>
              <div class="filter-group">
                <button v-for="filter in filters" :key="filter.value"
                  :class="{ active: statusFilter === filter.value }" @click="statusFilter = filter.value">
                  {{ filter.label }}
                </button>
              </div>
            </div>

            <div v-if="filteredIncidents.length" class="incident-table">
              <button v-for="incident in filteredIncidents" :key="incident.id"
                :class="['incident-row', selectedId === incident.id && 'selected']"
                @click="selectIncident(incident.id)">
                <span :class="['severity', `severity-${incident.severity}`]">{{ incident.severity }}</span>
                <span class="incident-main">
                  <strong>{{ incident.service }}</strong>
                  <small>{{ incident.description }}</small>
                </span>
                <span class="incident-meta">
                  <span :class="['state-badge', `state-${incident.status}`]">{{ statusLabel(incident.status) }}</span>
                  <small>{{ relativeTime(incident.updated_at || incident.created_at) }}</small>
                </span>
                <el-icon><ArrowRight /></el-icon>
              </button>
            </div>
            <div v-else class="empty-block">
              <div class="empty-icon"><el-icon><Search /></el-icon></div>
              <strong>当前筛选下没有事件</strong>
              <span>注入一个模拟故障，观察完整自治响应链路。</span>
              <el-button @click="showTrigger = true">创建演示事件</el-button>
            </div>
          </article>

          <article class="panel detail-panel">
            <template v-if="selectedIncident">
              <div class="panel-title detail-heading">
                <div>
                  <span class="eyebrow">事件 {{ shortId(selectedIncident.id) }}</span>
                  <h3>{{ selectedIncident.service }}</h3>
                </div>
                <span :class="['state-badge', `state-${selectedIncident.status}`]">{{ statusLabel(selectedIncident.status) }}</span>
              </div>

              <div class="incident-summary">
                <span :class="['severity large', `severity-${selectedIncident.severity}`]">{{ selectedIncident.severity }}</span>
                <div><strong>{{ selectedIncident.description }}</strong><small>创建于 {{ fullTime(selectedIncident.created_at) }}</small></div>
              </div>

              <div class="stage-track">
                <div v-for="(stage, index) in stages" :key="stage.key"
                  :class="['stage', stageState(stage.key)]">
                  <span>{{ index + 1 }}</span><small>{{ stage.label }}</small>
                </div>
              </div>

              <div :class="['event-conclusion', `conclusion-${selectedIncident.status}`]">
                <el-icon><CircleCheck /></el-icon>
                <div><strong>本次事件总结结论</strong><p>{{ selectedIncident.conclusion || selectedIncident.diagnosis?.summary || '事件处理中，完成后将在这里给出明确结论。' }}</p></div>
              </div>

              <div v-if="selectedIncident.diagnosis" class="diagnosis-card">
                <div class="diagnosis-top">
                  <div><span class="eyebrow accent">根因结论</span><h4>{{ selectedIncident.diagnosis.root_cause }}</h4></div>
                  <div class="confidence"><strong>{{ confidencePercent }}%</strong><small>置信度</small></div>
                </div>
                <p>{{ selectedIncident.diagnosis.summary }}</p>
                <div class="fix-plan">
                  <el-icon><MagicStick /></el-icon>
                  <div><span>建议动作 · {{ fixTypeLabel(selectedIncident.diagnosis.fix_type) }}</span>
                    <p>{{ selectedIncident.diagnosis.suggested_fix }}</p></div>
                </div>
              </div>
              <div v-else class="diagnosis-pending">
                <div class="pulse-orb"></div>
                <div><strong>{{ pendingText }}</strong><span>诊断智能体会依次检查指标、集群事件和工作负载日志。</span></div>
              </div>

              <div class="evidence-section">
                <div class="section-heading chain-heading">
                  <div><h4>端到端处理链路</h4><small>点击任一节点查看输入、处理逻辑与输出</small></div>
                  <span>{{ chainNodes.length }} 个节点</span>
                </div>
                <div v-if="chainNodes.length" class="trace-list">
                  <details v-for="(node, index) in chainNodes" :key="node.id"
                    :class="['trace-node', `trace-${node.kind}`, `trace-${node.status}`]" :open="node.defaultOpen">
                    <summary>
                      <span class="trace-index">{{ String(index + 1).padStart(2, '0') }}</span>
                      <span class="trace-marker"><el-icon><component :is="node.icon" /></el-icon></span>
                      <span class="trace-title">
                        <strong>{{ node.title }}</strong>
                        <small>{{ node.subtitle }}</small>
                      </span>
                      <span v-if="node.latency !== null" class="trace-latency">{{ node.latency }} ms</span>
                      <span :class="['trace-status', node.status]">{{ node.statusLabel }}</span>
                      <span class="trace-chevron">⌄</span>
                    </summary>
                    <div class="trace-body">
                      <p v-if="node.description" class="trace-description">{{ node.description }}</p>
                      <div v-if="node.meta?.length" class="trace-meta-grid">
                        <div v-for="item in node.meta" :key="item.label">
                          <span>{{ item.label }}</span><strong>{{ item.value }}</strong>
                        </div>
                      </div>
                      <section v-for="block in node.blocks" :key="block.label" class="trace-block">
                        <div class="trace-block-title">
                          <span>{{ block.label }}</span><small>{{ block.hint }}</small>
                        </div>
                        <pre>{{ block.content }}</pre>
                      </section>
                    </div>
                  </details>
                </div>
                <div v-else class="compact-empty">等待链路节点产生数据</div>
              </div>
            </template>
            <div v-else class="empty-block detail-empty">
              <div class="empty-icon"><el-icon><View /></el-icon></div>
              <strong>选择一个事件查看任务详情</strong>
              <span>这里会展示状态流转、根因、证据和修复策略。</span>
            </div>
          </article>
        </section>

        <section class="panel live-panel">
          <div class="panel-title">
            <div><span class="eyebrow">实时遥测</span><h3>实时事件总线</h3></div>
            <button class="text-button" @click="liveEvents = []">清空记录</button>
          </div>
          <div class="event-stream">
            <div v-for="(event, index) in liveEvents.slice(-12).reverse()" :key="`${event.timestamp}-${index}`" class="event-line">
              <span class="event-time">{{ timeOnly(event.timestamp) }}</span>
              <span :class="['event-dot', eventTone(event.type)]"></span>
              <strong>{{ eventLabel(event.type) }}</strong>
              <span>{{ eventSummary(event) }}</span>
            </div>
            <div v-if="!liveEvents.length" class="compact-empty">WebSocket 已连接，等待新的系统事件…</div>
          </div>
        </section>
      </main>

      <main v-else-if="currentView === 'history'">
        <section class="page-intro">
          <div><span class="eyebrow accent">SQLite 持久化</span><h2>历史事件与处理时间轴</h2><p>事件、模型步骤、状态流转和修复记录在服务重启后仍可查询与回放。</p></div>
          <span class="archive-count">{{ incidents.length }} 个持久化事件</span>
        </section>
        <section class="history-grid">
          <article class="panel archive-panel">
            <div class="panel-title"><div><span class="eyebrow">事件档案</span><h3>历史记录</h3></div></div>
            <div class="incident-table history-list">
              <button v-for="incident in incidents" :key="incident.id" :class="['incident-row', selectedId === incident.id && 'selected']" @click="selectHistoryIncident(incident.id)">
                <span :class="['severity', `severity-${incident.severity}`]">{{ incident.severity }}</span>
                <span class="incident-main"><strong>{{ incident.service }}</strong><small>{{ incident.description }}</small></span>
                <span class="incident-meta"><span :class="['state-badge', `state-${incident.status}`]">{{ statusLabel(incident.status) }}</span><small>{{ fullTime(incident.created_at) }}</small></span>
                <el-icon><ArrowRight /></el-icon>
              </button>
            </div>
          </article>
          <article class="panel timeline-panel">
            <div class="panel-title">
              <div><span class="eyebrow">处理轨迹</span><h3>{{ selectedIncident?.service || '请选择事件' }}</h3></div>
              <el-button v-if="selectedIncident" size="small" type="primary" @click="startReplay"><el-icon><VideoPlay /></el-icon> 诊断回放</el-button>
            </div>
            <div v-if="timeline.length" class="timeline-list">
              <div v-for="entry in timeline" :key="entry.id" class="timeline-entry">
                <span class="timeline-dot"></span>
                <div><strong>{{ entry.title }}</strong><p>{{ entry.description }}</p><small>{{ fullTime(entry.created_at) }} · {{ eventLabel(entry.event_type) }}</small></div>
              </div>
            </div>
            <div v-else class="empty-block"><div class="empty-icon"><el-icon><Clock /></el-icon></div><strong>暂无时间轴数据</strong><span>新产生的事件会记录完整处理轨迹。</span></div>
          </article>
        </section>
      </main>

      <main v-else>
        <section class="page-intro">
          <div><span class="eyebrow accent">单机工程实验室</span><h2>告警风暴与失败回滚演练</h2><p>不依赖真实集群，验证告警去重、队列吞吐量以及自动修复失败后的安全回滚。</p></div>
        </section>
        <section class="lab-grid">
          <article class="panel lab-card">
            <div class="panel-title"><div><span class="eyebrow">压力测试</span><h3>告警风暴发生器</h3></div></div>
            <div class="lab-form">
              <label>告警数量 <strong>{{ stormForm.count }}</strong></label>
              <el-slider v-model="stormForm.count" :min="100" :max="5000" :step="100" />
              <label>重复比例 <strong>{{ Math.round(stormForm.duplicateRatio * 100) }}%</strong></label>
              <el-slider v-model="stormForm.duplicateRatio" :min="0" :max="0.98" :step="0.05" />
              <el-button type="primary" :loading="stormRunning" @click="runStorm"><el-icon><Lightning /></el-icon> 开始告警风暴压测</el-button>
            </div>
          </article>
          <article class="panel lab-card">
            <div class="panel-title"><div><span class="eyebrow">压测结果</span><h3>去重与吞吐统计</h3></div></div>
            <div v-if="stormResult" class="storm-results">
              <div><span>原始告警</span><strong>{{ stormResult.requested }}</strong></div>
              <div><span>进入队列</span><strong>{{ stormResult.accepted }}</strong></div>
              <div><span>成功去重</span><strong>{{ stormResult.deduplicated }}</strong></div>
              <div><span>每秒吞吐</span><strong>{{ Math.round(stormResult.throughput_per_second) }}</strong></div>
              <p>耗时 {{ stormResult.elapsed_ms }} ms · 丢弃 {{ stormResult.dropped }} 条</p>
            </div>
            <div v-else class="empty-block"><div class="empty-icon"><el-icon><DataAnalysis /></el-icon></div><strong>等待执行压测</strong><span>结果会显示去重数量和入口吞吐量。</span></div>
          </article>
          <article class="panel lab-card rollback-card">
            <div><span class="eyebrow">回滚演示</span><h3>如何验证失败回滚</h3><p>打开“注入故障”，选择自动修复场景并将修复结果设为失败。系统会执行修复、发现健康状态未恢复、调用回滚并转交人工，所有过程都会写入时间轴。</p></div>
            <el-button @click="showTrigger = true">创建失败回滚事件</el-button>
          </article>
        </section>
      </main>
    </div>

    <el-dialog v-model="showTrigger" width="520px" class="fault-dialog" :show-close="false">
      <template #header>
        <div class="dialog-heading">
          <div class="dialog-icon"><el-icon><Lightning /></el-icon></div>
          <div><span class="eyebrow accent">故障注入</span><h3>注入一个可观测故障</h3>
            <p>模拟基础设施会同步生成指标、Kubernetes 事件和日志；带“自动修复”标记的场景可演示完整闭环。</p></div>
        </div>
      </template>
      <div class="scenario-grid">
        <button v-for="scenario in scenarios" :key="scenario.value"
          :class="{ selected: triggerForm.scenario === scenario.value }"
          @click="triggerForm.scenario = scenario.value">
          <el-icon><component :is="scenario.icon" /></el-icon>
          <span><strong>{{ scenario.label }}</strong><small>{{ scenario.detail }}</small></span>
          <em v-if="scenario.autoFix">自动修复</em>
        </button>
      </div>
      <label class="field-label">目标服务</label>
      <el-select v-model="triggerForm.service" size="large" style="width:100%">
        <el-option v-for="service in services" :key="service" :label="service" :value="service" />
      </el-select>
      <div class="outcome-field"><div><strong>模拟修复失败</strong><small>仅自动修复场景生效，用于演示验证失败和回滚</small></div><el-switch v-model="triggerForm.failRepair" /></div>
      <template #footer>
        <el-button @click="showTrigger = false">取消</el-button>
        <el-button type="primary" :loading="triggering" @click="doTrigger">
          <el-icon><Lightning /></el-icon> 开始故障演练
        </el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="showReplay" width="680px" class="fault-dialog replay-dialog" title="诊断过程回放" @closed="stopReplay">
      <div class="replay-toolbar">
        <span>第 {{ replayIndex }} / {{ replayFrames.length }} 帧</span>
        <el-button size="small" @click="toggleReplay"><el-icon><VideoPause v-if="replayPlaying" /><VideoPlay v-else /></el-icon>{{ replayPlaying ? '暂停' : '继续' }}</el-button>
      </div>
      <el-progress :percentage="replayProgress" :show-text="false" />
      <div class="replay-stage">
        <div v-for="entry in replayFrames.slice(0, replayIndex)" :key="entry.id" class="timeline-entry replay-entry">
          <span class="timeline-dot"></span><div><strong>{{ entry.title }}</strong><p>{{ entry.description }}</p><small>{{ eventLabel(entry.event_type) }}</small></div>
        </div>
      </div>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { ElButton, ElDialog, ElIcon, ElMessage, ElOption, ElSelect } from 'element-plus'
import {
  Aim, ArrowRight, Bell, CircleCheck, Clock, Connection, Cpu, DataAnalysis,
  Lightning, Lock, MagicStick, Moon, Operation, Refresh, Search, Sunny, Tickets, View, Warning,
  VideoPause, VideoPlay,
} from '@element-plus/icons-vue'
import { fetchHealth, fetchIncident, fetchIncidents, fetchReplay, fetchTimeline, triggerAlert, triggerStorm, wsClient } from './api.js'

const incidents = ref([])
const currentView = ref('console')
const selectedId = ref(null)
const statusFilter = ref('all')
const wsConnected = ref(false)
const liveEvents = ref([])
const streamSteps = reactive({})
const showTrigger = ref(false)
const triggering = ref(false)
const timeline = ref([])
const showReplay = ref(false)
const replayFrames = ref([])
const replayIndex = ref(0)
const replayPlaying = ref(false)
const stormRunning = ref(false)
const stormResult = ref(null)
const stormForm = reactive({ count: 1000, duplicateRatio: 0.8, service: 'storm-demo' })
const health = reactive({ gateway: 'checking', brain: 'checking' })
const triggerForm = reactive({ scenario: 'crash_loop', service: 'payment', failRepair: false })
const theme = ref(getInitialTheme())

applyTheme(theme.value)

const filters = [
  { label: '全部', value: 'all' },
  { label: '活跃', value: 'active' },
  { label: 'P0', value: 'P0' },
  { label: '已升级', value: 'escalated' },
]
const services = ['payment', 'order', 'user-service', 'gateway', 'notification']
const scenarios = [
  { value: 'oom_kill', label: '内存耗尽', detail: '最高优先级 · 修改配置需人工审核', icon: Cpu },
  { value: 'high_cpu', label: 'CPU 飙升', detail: '高优先级 · 自动扩容', icon: DataAnalysis, autoFix: true },
  { value: 'high_latency', label: '请求变慢', detail: '高优先级 · 配置变更需人工审核', icon: Connection },
  { value: 'disk_full', label: '磁盘压力', detail: '普通优先级 · 清理操作需确认', icon: Warning },
  { value: 'conn_timeout', label: '依赖超时', detail: '最高优先级 · 配置变更需人工审核', icon: Connection },
  { value: 'crash_loop', label: '反复崩溃', detail: '最高优先级 · 自动重启并验证', icon: Bell, autoFix: true },
]
const capabilityFlow = [
  { label: '接入', detail: '告警标准化', icon: Bell },
  { label: '聚合', detail: '事件关联', icon: DataAnalysis },
  { label: '诊断', detail: '证据推理', icon: Cpu },
  { label: '处置', detail: '安全护栏', icon: MagicStick },
]
const stages = [
  { key: 'new', label: '接收' },
  { key: 'diagnosing', label: '诊断' },
  { key: 'diagnosed', label: '决策' },
  { key: 'fixing', label: '修复' },
  { key: 'resolved', label: '恢复' },
]
const stateOrder = ['new', 'diagnosing', 'diagnosed', 'fixing', 'fixed', 'resolved']
const terminalStates = ['resolved', 'escalated', 'false_alarm']

const selectedIncident = computed(() => incidents.value.find((item) => item.id === selectedId.value) || null)
const viewTitle = computed(() => ({ console: '智能运维任务中心', history: '历史事件档案', lab: '演练与压测实验室' }[currentView.value]))
const replayProgress = computed(() => replayFrames.value.length ? Math.round((replayIndex.value / replayFrames.value.length) * 100) : 0)
const filteredIncidents = computed(() => incidents.value.filter((item) => {
  if (statusFilter.value === 'all') return true
  if (statusFilter.value === 'active') return !terminalStates.includes(item.status)
  if (statusFilter.value === 'P0') return item.severity === 'P0'
  return item.status === statusFilter.value
}))
const activeCount = computed(() => incidents.value.filter((item) => !terminalStates.includes(item.status)).length)
const criticalCount = computed(() => incidents.value.filter((item) => ['P0', 'P1'].includes(item.severity)).length)
const resolvedCount = computed(() => incidents.value.filter((item) => item.status === 'resolved').length)
const automationRate = computed(() => incidents.value.length ? Math.round((resolvedCount.value / incidents.value.length) * 100) : 0)
const confidencePercent = computed(() => Math.round((selectedIncident.value?.diagnosis?.confidence || 0) * 100))
const visibleSteps = computed(() => {
  const persisted = selectedIncident.value?.diagnosis?.diagnosis_path || []
  return persisted.length ? persisted : (streamSteps[selectedId.value] || [])
})
const chainNodes = computed(() => buildChainNodes(selectedIncident.value, visibleSteps.value))
const pendingText = computed(() => {
  const status = selectedIncident.value?.status
  if (status === 'escalated') return '已升级人工处置'
  if (status === 'new') return '等待进入诊断队列'
  return '诊断智能体正在构建证据链'
})

function statusLabel(status) {
  return {
    new: '新建', diagnosing: '诊断中', diagnosed: '已诊断', fixing: '自动修复',
    fixed: '观察中', resolved: '已恢复', escalated: '人工接管', false_alarm: '误报',
  }[status] || status
}

function getInitialTheme() {
  try {
    const saved = window.localStorage.getItem('watchtower-theme')
    if (saved === 'light' || saved === 'dark') return saved
  } catch {
    // Storage can be unavailable in privacy-restricted browser contexts.
  }
  return window.matchMedia?.('(prefers-color-scheme: light)').matches ? 'light' : 'dark'
}

function applyTheme(value) {
  document.documentElement.dataset.theme = value
  document.documentElement.style.colorScheme = value
}

function toggleTheme() {
  theme.value = theme.value === 'dark' ? 'light' : 'dark'
  applyTheme(theme.value)
  try {
    window.localStorage.setItem('watchtower-theme', theme.value)
  } catch {
    // The active session still switches theme even if persistence is blocked.
  }
}

function stageState(stage) {
  const current = selectedIncident.value?.status
  if (current === 'escalated') return stage === 'diagnosed' ? 'current warning' : ''
  const currentIndex = stateOrder.indexOf(current)
  const stageIndex = stateOrder.indexOf(stage)
  if (stage === 'resolved' && current === 'fixed') return ''
  if (stageIndex < currentIndex) return 'complete'
  if (stageIndex === currentIndex || (stage === 'resolved' && current === 'resolved')) return 'current'
  return ''
}

function upsertIncident(incident) {
  if (!incident?.id) return
  const index = incidents.value.findIndex((item) => item.id === incident.id)
  if (index >= 0) incidents.value[index] = incident
  else incidents.value.unshift(incident)
  incidents.value.sort((a, b) => new Date(b.created_at) - new Date(a.created_at))
  if (!selectedId.value) selectedId.value = incident.id
}

async function loadIncidents() {
  try {
    const data = await fetchIncidents()
    incidents.value = data.incidents || []
    if (!selectedId.value && incidents.value.length) selectedId.value = incidents.value[0].id
  } catch {
    health.gateway = 'unavailable'
  }
}

async function loadHealth() {
  try {
    const data = await fetchHealth()
    health.gateway = data.gateway || (data.healthy ? 'ok' : 'unavailable')
    health.brain = data.brain || 'unavailable'
  } catch {
    health.gateway = 'unavailable'
    health.brain = 'unavailable'
  }
}

async function selectIncident(id) {
  selectedId.value = id
  try {
    upsertIncident(await fetchIncident(id))
  } catch {
    ElMessage.warning('事件详情暂时不可用')
  }
}

async function openHistory() {
  currentView.value = 'history'
  await loadIncidents()
  if (selectedId.value) await loadTimeline(selectedId.value)
}

async function selectHistoryIncident(id) {
  await selectIncident(id)
  await loadTimeline(id)
}

async function loadTimeline(id) {
  try {
    timeline.value = (await fetchTimeline(id)).timeline || []
  } catch {
    timeline.value = []
    ElMessage.warning('事件时间轴暂时不可用')
  }
}

let replayTimer
async function startReplay() {
  if (!selectedId.value) return
  try {
    const data = await fetchReplay(selectedId.value)
    replayFrames.value = data.frames || []
    replayIndex.value = 0
    showReplay.value = true
    replayPlaying.value = true
    scheduleReplay()
  } catch {
    ElMessage.error('回放数据加载失败')
  }
}

function scheduleReplay() {
  clearInterval(replayTimer)
  if (!replayPlaying.value) return
  replayTimer = setInterval(() => {
    if (replayIndex.value >= replayFrames.value.length) {
      replayPlaying.value = false
      clearInterval(replayTimer)
      return
    }
    replayIndex.value += 1
  }, 700)
}

function toggleReplay() {
  if (replayIndex.value >= replayFrames.value.length) replayIndex.value = 0
  replayPlaying.value = !replayPlaying.value
  scheduleReplay()
}

function stopReplay() {
  replayPlaying.value = false
  clearInterval(replayTimer)
}

async function runStorm() {
  stormRunning.value = true
  try {
    stormResult.value = await triggerStorm(stormForm)
    ElMessage.success('告警风暴压测完成')
    setTimeout(loadIncidents, 500)
  } catch (error) {
    ElMessage.error(error.message || '压测执行失败')
  } finally {
    stormRunning.value = false
  }
}

async function refreshAll() {
  await Promise.all([loadIncidents(), loadHealth()])
  ElMessage.success('态势数据已刷新')
}

async function doTrigger() {
  triggering.value = true
  try {
    await triggerAlert(triggerForm.scenario, triggerForm.service, triggerForm.failRepair ? 'failure' : 'success')
    showTrigger.value = false
    ElMessage.success('故障已注入，诊断智能体开始响应')
    setTimeout(loadIncidents, 350)
  } catch (error) {
    ElMessage.error(error.message || '故障注入失败')
  } finally {
    triggering.value = false
  }
}

function onWsConnected(value) {
  wsConnected.value = value
}

function onWsMessage(event) {
  liveEvents.value.push(event)
  if (liveEvents.value.length > 80) liveEvents.value.shift()

  const payload = event.payload || {}
  if (event.type === 'incident_escalated') upsertIncident(payload.incident)
  else if (event.type === 'diagnosis_step') {
    if (!streamSteps[payload.incident_id]) streamSteps[payload.incident_id] = []
    const exists = streamSteps[payload.incident_id].some((step) => step.step_number === payload.step_number)
    if (!exists) streamSteps[payload.incident_id].push(payload)
  } else if (event.type === 'diagnosis_complete') {
    loadIncidents()
  } else if (event.type.startsWith('incident_') || event.type.startsWith('autofix_') || event.type === 'new_incident') {
    upsertIncident(payload)
  }
}

function shortId(id) { return id?.slice(0, 8).toUpperCase() }
function fullTime(value) { return value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '—' }
function timeOnly(value) { return value ? new Date(value).toLocaleTimeString('zh-CN', { hour12: false }) : '—' }
function relativeTime(value) {
  if (!value) return '—'
  const seconds = Math.max(0, Math.floor((Date.now() - new Date(value).getTime()) / 1000))
  if (seconds < 60) return `${seconds} 秒前`
  if (seconds < 3600) return `${Math.floor(seconds / 60)} 分钟前`
  return `${Math.floor(seconds / 3600)} 小时前`
}
function eventLabel(type) {
  return {
    new_incident: '事件创建', incident_updated: '事件聚合', incident_status_changed: '状态流转',
    diagnosis_step: '诊断证据', diagnosis_complete: '诊断完成', incident_escalated: '人工升级',
    autofix_started: '开始修复', autofix_completed: '修复完成', incident_resolved: '事件恢复',
  }[type] || type
}
function eventTone(type) {
  if (type.includes('escalated')) return 'danger'
  if (type.includes('resolved') || type.includes('completed')) return 'success'
  if (type.includes('diagnosis')) return 'violet'
  return 'blue'
}
function eventSummary(event) {
  const payload = event.payload?.incident || event.payload || {}
  if (event.type === 'diagnosis_step') {
    const trace = parseTrace(payload.observation)
    if (trace?.trace_type === 'llm') return `${trace.provider}/${trace.model} · ${traceModeLabel(trace.mode)}`
    if (trace?.trace_type === 'rule') return '无可用 LLM，规则引擎生成下一步'
    if (trace?.trace_type === 'tool') return `${toolLabel(trace.tool_name)} · ${summarize(trace.output)}`
    return `${payload.action}: ${payload.observation || ''}`.slice(0, 120)
  }
  if (event.type === 'diagnosis_complete') return payload.summary || payload.root_cause || '已生成诊断结论'
  return [payload.service, payload.description || payload.reason, statusLabel(payload.status)].filter(Boolean).join(' · ')
}

function buildChainNodes(incident, steps) {
  if (!incident) return []
  const alert = incident.root_alert || {}
  const nodes = [
    {
      id: `alert-${incident.id}`, kind: 'ingress', status: 'success', statusLabel: '已接收', icon: Bell,
      title: '告警接入', subtitle: `${sourceLabel(alert.source)} → 告警网关`, latency: null,
      description: '原始告警进入统一入口，随后转换为内部标准结构。', defaultOpen: false,
      meta: [
        { label: '告警名称', value: alert.alert_name || '—' },
        { label: '目标服务', value: incident.service || '—' },
        { label: '严重级别', value: incident.severity || '—' },
        { label: '接收时间', value: fullTime(alert.timestamp || incident.created_at) },
      ],
      blocks: [{ label: '节点输入 / 标准化结果', hint: '告警数据', content: prettyJson(alert) }],
    },
    {
      id: `gateway-${incident.id}`, kind: 'gateway', status: 'success', statusLabel: '已通过', icon: DataAnalysis,
      title: '告警网关预处理', subtitle: '去重 → 静默 → 优先级队列 → 事件关联', latency: null,
      description: '该事件已成功创建，说明告警没有被去重、静默或队列容量规则丢弃。', defaultOpen: false,
      meta: [
        { label: '去重结果', value: '通过' }, { label: '静默结果', value: '通过' },
        { label: '队列优先级', value: incident.severity || '—' },
        { label: '关联告警', value: `${incident.related_alerts?.length || 1} 条` },
      ],
      blocks: [{ label: '节点输出', hint: '事件初始数据', content: prettyJson({
        service: incident.service, severity: incident.severity, description: incident.description,
        related_alert_count: incident.related_alerts?.length || 1,
      }) }],
    },
    {
      id: `incident-${incident.id}`, kind: 'incident', status: terminalStates.includes(incident.status) ? 'success' : 'running',
      statusLabel: terminalStates.includes(incident.status) ? '已流转' : '进行中', icon: Tickets,
      title: '事件编排与状态机', subtitle: `当前状态：${statusLabel(incident.status)}`, latency: null,
      description: '事件管理器按照严重级别启动诊断或观察，并限制非法状态跳转。', defaultOpen: false,
      meta: [
        { label: '事件 ID', value: incident.id }, { label: '当前状态', value: statusLabel(incident.status) },
        { label: '创建时间', value: fullTime(incident.created_at) }, { label: '更新时间', value: fullTime(incident.updated_at) },
      ],
      blocks: [{ label: '节点输出', hint: '事件状态快照', content: prettyJson({
        id: incident.id, status: incident.status, service: incident.service, severity: incident.severity,
        created_at: incident.created_at, updated_at: incident.updated_at,
      }) }],
    },
  ]

  steps.forEach((step, index) => nodes.push(stepToNode(step, index, incident.id)))

  if (incident.diagnosis) {
    nodes.push({
      id: `diagnosis-${incident.id}`, kind: 'decision', status: 'success', statusLabel: '已决策', icon: CircleCheck,
      title: '诊断结论', subtitle: `${Math.round((incident.diagnosis.confidence || 0) * 100)}% 置信度`, latency: null,
      description: incident.diagnosis.summary || '已根据证据链生成结构化诊断。', defaultOpen: false,
      meta: [
        { label: '根因', value: incident.diagnosis.root_cause || '—' },
        { label: '修复类型', value: fixTypeLabel(incident.diagnosis.fix_type) },
        { label: '允许自动修复', value: incident.diagnosis.auto_fixable ? '是' : '否' },
        { label: '需要升级', value: incident.diagnosis.escalated ? '是' : '否' },
      ],
      blocks: [
        { label: '证据', hint: '诊断证据', content: prettyJson(incident.diagnosis.evidence || []) },
        { label: '建议动作', hint: '修复建议', content: incident.diagnosis.suggested_fix || '无' },
        { label: '完整输出', hint: '诊断结果数据', content: prettyJson(incident.diagnosis) },
      ],
    })
  }

  if (['fixing', 'fixed', 'resolved', 'escalated'].includes(incident.status)) {
    const escalated = incident.status === 'escalated'
    nodes.push({
      id: `disposition-${incident.id}`, kind: escalated ? 'escalation' : 'autofix',
      status: escalated ? 'warning' : incident.status === 'resolved' ? 'success' : 'running',
      statusLabel: escalated ? '人工接管' : statusLabel(incident.status), icon: escalated ? Warning : MagicStick,
      title: escalated ? '安全护栏与人工升级' : '自动修复与观察',
      subtitle: escalated ? '风险规则阻止自动执行' : '当前执行器为模拟模式', latency: null,
      description: escalated ? '诊断结果未满足自动修复条件，系统停止自动操作并升级人工。' : '风险引擎通过后进入模拟执行、验证和观察流程。',
      defaultOpen: false,
      meta: [{ label: '最终状态', value: statusLabel(incident.status) }, { label: 'MTTR', value: `${incident.mttr_seconds || 0} 秒` }],
      blocks: [{ label: '处置输入', hint: '安全策略判断', content: prettyJson({
        修复类型: fixTypeLabel(incident.diagnosis?.fix_type), 置信度: incident.diagnosis?.confidence,
        允许自动修复: incident.diagnosis?.auto_fixable ? '是' : '否', 事件状态: statusLabel(incident.status),
      }) }],
    })
  }
  return nodes
}

function stepToNode(step, index, incidentId) {
  const trace = parseTrace(step.observation)
  const base = {
    id: `trace-${incidentId}-${step.step_number ?? index}-${index}`,
    latency: Number.isFinite(Number(step.latency_ms)) ? Number(step.latency_ms) : 0,
    description: step.thought || '', meta: [], blocks: [], defaultOpen: false,
  }
  if (trace?.trace_type === 'llm') {
    const failed = trace.mode === 'fallback_after_error'
    return {
      ...base, kind: 'llm', status: failed ? 'warning' : 'success', statusLabel: failed ? '已降级' : '模型已返回',
      icon: Cpu, title: '大模型推理决策', subtitle: `${trace.provider || '大模型'} / ${trace.model || '未知模型'}`,
      defaultOpen: true,
      meta: [
        { label: '运行模式', value: traceModeLabel(trace.mode) }, { label: '接口地址', value: trace.endpoint || '—' },
        { label: '模型', value: trace.model || '—' }, { label: '耗时', value: `${trace.latency_ms || 0} ms` },
      ],
      blocks: [
        { label: '发送给模型的输入', hint: '认证信息已在服务端隐藏', content: prettyJson(trace.input) },
        { label: '模型原始输出', hint: failed ? '调用失败并已降级' : '原始响应和解析结果', content: prettyJson(trace.output) },
      ],
    }
  }
  if (trace?.trace_type === 'rule') {
    return {
      ...base, kind: 'rule', status: 'warning', statusLabel: '规则降级', icon: Operation,
      title: '规则推理决策', subtitle: '大模型未启用或未配置接口密钥',
      meta: [{ label: '运行模式', value: traceModeLabel(trace.mode) }, { label: '模型配置', value: trace.model || '—' }],
      blocks: [
        { label: '规则输入', hint: '诊断上下文', content: prettyJson(trace.input) },
        { label: '规则输出', hint: '下一步动作', content: prettyJson(trace.output) },
      ],
    }
  }
  if (trace?.trace_type === 'tool') {
    return {
      ...base, kind: 'tool', status: trace.output?.error ? 'warning' : 'success',
      statusLabel: trace.output?.error ? '查询异常' : '查询完成', icon: Search,
      title: toolLabel(trace.tool_name), subtitle: '诊断工具调用',
      meta: [{ label: '工具名称', value: trace.tool_name || '—' }, { label: '耗时', value: `${base.latency} ms` }],
      blocks: [
        { label: '工具输入', hint: '查询参数', content: prettyJson(trace.input) },
        { label: '工具输出', hint: '观察结果', content: prettyJson(trace.output) },
      ],
    }
  }
  return {
    ...base, kind: 'tool', status: 'success', statusLabel: '已记录', icon: Search,
    title: step.action || '诊断步骤', subtitle: '兼容旧版本追踪数据',
    blocks: [{ label: '节点输出', hint: '原始观察结果', content: step.observation || '无' }],
  }
}

function parseTrace(value) {
  if (!value || typeof value !== 'string') return null
  try {
    const parsed = JSON.parse(value)
    return parsed && typeof parsed === 'object' ? parsed : null
  } catch { return null }
}

function prettyJson(value) {
  if (typeof value === 'string') {
    try { return JSON.stringify(JSON.parse(value), null, 2) } catch { return value }
  }
  return JSON.stringify(value ?? null, null, 2)
}

function summarize(value) {
  if (value == null) return '无输出'
  if (typeof value === 'string') return value.slice(0, 80)
  return Object.keys(value).slice(0, 4).join(', ') || '已返回数据'
}

function toolLabel(name) {
  return {
    prometheus_query: 'Prometheus 指标查询',
    k8s_get_events: 'Kubernetes 事件查询',
    k8s_get_logs: 'Pod 日志查询',
    similar_incidents: '历史故障检索',
  }[name] || `诊断工具 · ${name || '未知工具'}`
}

function fixTypeLabel(type) {
  return {
    restart_pod: '自动重启 Pod', scale: '自动扩缩容', cleanup_disk: '清理磁盘',
    config_change: '修改配置', traffic_switch: '切换流量', manual_review: '人工审核',
  }[type] || '人工审核'
}

function traceModeLabel(mode) {
  return {
    live: '模型实时推理', rule_fallback: '规则降级', fallback_after_error: '模型异常后规则降级',
  }[mode] || '诊断处理中'
}

function sourceLabel(source) {
  return { mock: '模拟告警', custom: '自定义告警', prometheus: '监控告警' }[source] || '未知来源'
}

let refreshTimer
onMounted(() => {
  wsClient.on('connected', onWsConnected)
  wsClient.on('message', onWsMessage)
  wsClient.connect()
  loadIncidents()
  loadHealth()
  refreshTimer = setInterval(() => {
    loadIncidents()
    loadHealth()
  }, 10_000)
})

onUnmounted(() => {
  clearInterval(refreshTimer)
  clearInterval(replayTimer)
  wsClient.off('connected', onWsConnected)
  wsClient.off('message', onWsMessage)
  wsClient.close()
})
</script>
