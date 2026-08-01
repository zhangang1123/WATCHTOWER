package mock

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// K8sServer 模拟 Kubernetes API
type K8sServer struct {
	registry *ScenarioRegistry
}

// NewK8sServer 创建模拟 K8s 服务
func NewK8sServer(registries ...*ScenarioRegistry) *K8sServer {
	registry := NewScenarioRegistry()
	if len(registries) > 0 && registries[0] != nil {
		registry = registries[0]
	}
	return &K8sServer{registry: registry}
}

// RegisterRoutes 注册路由
func (s *K8sServer) RegisterRoutes(r *gin.Engine) {
	r.GET("/mock/k8s/events", s.handleEvents)
	r.GET("/mock/k8s/pods", s.handlePods)
	r.GET("/mock/k8s/logs", s.handleLogs)
}

// RegisterScenario 注册故障场景数据
func (s *K8sServer) RegisterScenario(service, scenario string) {
	s.registry.Register(service, scenario, "success")
}

func (s *K8sServer) handleEvents(c *gin.Context) {
	namespace := c.Query("namespace")
	resourceType := c.Query("resource_type")

	data, ok := s.registry.Snapshot(namespace)
	if !ok {
		// 返回默认空事件
		c.JSON(http.StatusOK, []gin.H{})
		return
	}

	var events []gin.H
	for _, e := range data.Events {
		if resourceType != "" && !strings.Contains(e.Message, resourceType) {
			continue
		}
		events = append(events, gin.H{
			"type":    e.Type,
			"reason":  e.Reason,
			"message": e.Message,
		})
	}
	c.JSON(http.StatusOK, events)
}

func (s *K8sServer) handlePods(c *gin.Context) {
	namespace := c.Query("namespace")

	data, ok := s.registry.Snapshot(namespace)
	if !ok {
		c.JSON(http.StatusOK, []gin.H{
			{"name": namespace + "-7d9f4", "status": "运行中", "restarts": 0, "age": "3 天"},
		})
		return
	}

	status := "运行中"
	if data.State == StateRecovered {
		status = "运行中"
	} else if data.Scenario == "oom_kill" || data.Scenario == "crash_loop" {
		status = "反复崩溃并退避重启"
	}

	c.JSON(http.StatusOK, []gin.H{
		{"name": namespace + "-7d9f4", "status": status, "restarts": map[bool]int{true: 0, false: 3}[data.State == StateRecovered], "age": "3 天"},
	})
}

func (s *K8sServer) handleLogs(c *gin.Context) {
	namespace := c.Query("namespace")

	data, ok := s.registry.Snapshot(namespace)
	if !ok {
		c.JSON(http.StatusOK, []string{"信息：服务已启动", "信息：健康检查已通过"})
		return
	}

	c.JSON(http.StatusOK, data.Logs)
}
