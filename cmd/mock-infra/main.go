package main

import (
	"net/http"

	"watchtower-lite/internal/config"
	"watchtower-lite/internal/mock"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func main() {
	if err := config.Load(); err != nil {
		panic("load configuration: " + err.Error())
	}
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	// Mock Prometheus 服务
	promPort := config.Get("MOCK_PROMETHEUS_PORT", "9090")
	promRouter := gin.Default()
	registry := mock.NewScenarioRegistry()
	promServer := mock.NewPrometheusServer(registry)
	promServer.RegisterRoutes(promRouter)
	promRouter.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "mock-prometheus"})
	})

	go func() {
		logger.Info("Mock Prometheus starting", zap.String("port", promPort))
		if err := promRouter.Run(":" + promPort); err != nil {
			logger.Fatal("prometheus mock failed", zap.Error(err))
		}
	}()

	// Mock K8s 服务
	k8sPort := config.Get("MOCK_K8S_PORT", "8081")
	k8sRouter := gin.Default()
	k8sServer := mock.NewK8sServer(registry)
	k8sServer.RegisterRoutes(k8sRouter)
	k8sRouter.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "mock-k8s"})
	})
	k8sRouter.POST("/mock/scenarios", func(c *gin.Context) {
		var req struct {
			Service       string `json:"service" binding:"required"`
			Scenario      string `json:"scenario" binding:"required"`
			RepairOutcome string `json:"repair_outcome"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		data := registry.Register(req.Service, req.Scenario, req.RepairOutcome)
		c.JSON(http.StatusOK, gin.H{"registered": true, "scenario": data})
	})
	k8sRouter.GET("/mock/scenarios/:service", func(c *gin.Context) {
		data, ok := registry.Snapshot(c.Param("service"))
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "模拟场景不存在"})
			return
		}
		c.JSON(http.StatusOK, data)
	})
	k8sRouter.POST("/mock/actions", func(c *gin.Context) {
		var req struct {
			Service string `json:"service" binding:"required"`
			Action  string `json:"action" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		data, err := registry.ApplyAction(req.Service, req.Action)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"accepted": true, "scenario": data})
	})

	logger.Info("Mock K8s starting", zap.String("port", k8sPort))
	if err := k8sRouter.Run(":" + k8sPort); err != nil {
		logger.Fatal("k8s mock failed", zap.Error(err))
	}
}
