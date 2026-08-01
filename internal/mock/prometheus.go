package mock

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// PrometheusServer 模拟 Prometheus 服务
type PrometheusServer struct {
	registry *ScenarioRegistry
}

// NewPrometheusServer 创建模拟 Prometheus
func NewPrometheusServer(registries ...*ScenarioRegistry) *PrometheusServer {
	registry := NewScenarioRegistry()
	if len(registries) > 0 && registries[0] != nil {
		registry = registries[0]
	}
	return &PrometheusServer{registry: registry}
}

// RegisterRoutes 注册路由
func (s *PrometheusServer) RegisterRoutes(r *gin.Engine) {
	r.GET("/api/v1/query", s.handleQuery)
	r.GET("/api/v1/query_range", s.handleQueryRange)
}

// RegisterScenario 注册故障场景数据
func (s *PrometheusServer) RegisterScenario(service, scenario string) {
	s.registry.Register(service, scenario, "success")
}

func (s *PrometheusServer) handleQuery(c *gin.Context) {
	query := c.Query("query")
	service := extractService(query)

	data, ok := s.registry.Snapshot(service)
	if !ok {
		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   gin.H{"resultType": "vector", "result": []gin.H{}},
		})
		return
	}

	result := s.evaluateQuery(query, data)
	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   result,
	})
}

func (s *PrometheusServer) handleQueryRange(c *gin.Context) {
	// 简化版：返回空序列
	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   gin.H{"resultType": "matrix", "result": []gin.H{}},
	})
}

func (s *PrometheusServer) evaluateQuery(query string, data *ScenarioData) gin.H {
	result := []gin.H{}
	ts := float64(time.Now().Unix())

	switch {
	case strings.Contains(query, "up"):
		result = append(result, gin.H{
			"metric": gin.H{"service": data.Service, "__name__": "up"},
			"value":  []interface{}{ts, fmt.Sprintf("%d", int(data.Metrics["up"]))},
		})
	case strings.Contains(query, "cpu_usage"):
		result = append(result, gin.H{
			"metric": gin.H{"service": data.Service, "__name__": "container_cpu_usage_seconds_total"},
			"value":  []interface{}{ts, fmt.Sprintf("%.2f", data.Metrics["cpu_usage"])},
		})
	case strings.Contains(query, "memory_usage"):
		result = append(result, gin.H{
			"metric": gin.H{"service": data.Service, "__name__": "container_memory_working_set_bytes"},
			"value":  []interface{}{ts, fmt.Sprintf("%.0f", data.Metrics["memory_usage"])},
		})
	case strings.Contains(query, "http_requests_total"):
		result = append(result, gin.H{
			"metric": gin.H{"service": data.Service, "__name__": "http_requests_total"},
			"value":  []interface{}{ts, fmt.Sprintf("%.0f", data.Metrics["request_rate"]*60)},
		})
	case strings.Contains(query, "http_request_duration_seconds"):
		result = append(result, gin.H{
			"metric": gin.H{"service": data.Service, "__name__": "http_request_duration_seconds"},
			"value":  []interface{}{ts, fmt.Sprintf("%.3f", data.Metrics["latency_p99"])},
		})
	case strings.Contains(query, "error_rate"):
		result = append(result, gin.H{
			"metric": gin.H{"service": data.Service, "__name__": "error_rate"},
			"value":  []interface{}{ts, fmt.Sprintf("%.3f", data.Metrics["error_rate"])},
		})
	}

	return gin.H{"resultType": "vector", "result": result}
}

func generateScenarioData(service, scenario string) *ScenarioData {
	data := &ScenarioData{
		Service:   service,
		Scenario:  scenario,
		Metrics:   make(map[string]float64),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	switch scenario {
	case "oom_kill":
		data.Metrics["up"] = 0
		data.Metrics["memory_usage"] = 1073741824 // 1GB
		data.Events = []K8sEvent{
			{Type: "警告", Reason: "内存终止（OOMKilling）", Message: "容器内存控制组耗尽，系统正在终止进程"},
		}
		data.Logs = []string{
			"系统已终止进程 1234（payment-service），常驻内存为 1048576kB",
			"内存不足：进程 1234 的内存评分为 999，触发系统保护",
		}
	case "high_cpu":
		data.Metrics["up"] = 1
		data.Metrics["cpu_usage"] = 0.95
		data.Metrics["request_rate"] = 5000
		data.Events = []K8sEvent{
			{Type: "警告", Reason: "CPU 使用率过高", Message: "检测到 CPU 限流，工作负载计算资源不足"},
		}
	case "high_latency":
		data.Metrics["up"] = 1
		data.Metrics["latency_p99"] = 2.5 // 2.5s
		data.Metrics["error_rate"] = 0.15
		data.Events = []K8sEvent{
			{Type: "警告", Reason: "响应缓慢", Message: "请求响应时间持续超过告警阈值"},
		}
		data.Logs = []string{
			"错误：数据库连接池已耗尽",
			"警告：数据库查询在 30 秒后超时",
		}
	case "disk_full":
		data.Metrics["up"] = 1
		data.Metrics["disk_usage"] = 0.92
		data.Events = []K8sEvent{
			{Type: "警告", Reason: "磁盘压力", Message: "节点磁盘空间不足，已触发磁盘压力状态"},
		}
	case "conn_timeout":
		data.Metrics["up"] = 1
		data.Metrics["error_rate"] = 0.80
		data.Metrics["latency_p99"] = 10.0
		data.Logs = []string{
			"错误：连接 Redis（6379 端口）超时",
			"错误：连接 Redis（6379 端口）超时",
			"警告：第 3 次重试仍然失败",
		}
	case "crash_loop":
		data.Metrics["up"] = 0
		data.Events = []K8sEvent{
			{Type: "警告", Reason: "退避重启", Message: "容器启动失败，系统正在延长重启间隔"},
			{Type: "正常", Reason: "容器已创建", Message: "已创建 payment-service 容器"},
			{Type: "警告", Reason: "启动失败", Message: "容器因程序异常而崩溃"},
		}
		data.Logs = []string{
			"程序异常：数组访问越界（index out of range）",
			"主协程正在运行时发生异常",
			"异常位置：main.processPayment(...) 函数",
		}
	default:
		data.Metrics["up"] = 1
		data.Metrics["cpu_usage"] = 0.3
		data.Metrics["latency_p99"] = 0.05
		data.Metrics["error_rate"] = 0.001
	}

	return data
}

// K8sEvent K8s 事件
type K8sEvent struct {
	Type    string `json:"type"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

var serviceMatcher = regexp.MustCompile(`service\s*=\s*['"]([^'"]+)['"]`)

func extractService(query string) string {
	if match := serviceMatcher.FindStringSubmatch(query); len(match) == 2 {
		return match[1]
	}
	return "default"
}
