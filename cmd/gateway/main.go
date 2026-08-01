package main

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"watchtower-lite/internal/agent"
	"watchtower-lite/internal/autofix"
	"watchtower-lite/internal/config"
	"watchtower-lite/internal/gateway"
	"watchtower-lite/internal/incident"
	"watchtower-lite/internal/models"
	"watchtower-lite/internal/store"
	"watchtower-lite/internal/ws"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

func main() {
	if err := config.Load(); err != nil {
		panic("load configuration: " + err.Error())
	}
	// 初始化日志
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	// 初始化各组件
	hub := ws.NewHub(logger)
	go hub.Run()

	mockControlURL := config.Get("WATCHTOWER_MOCK_CONTROL_URL", "http://localhost:8081")
	autofixEngine := autofix.NewEngine(logger, mockControlURL)
	database, err := store.OpenSQLite(config.Get("WATCHTOWER_DB_PATH", "data/watchtower.db"))
	if err != nil {
		logger.Fatal("无法打开 SQLite 数据库", zap.Error(err))
	}
	defer database.Close()

	// Agent 客户端
	brainAddr := config.Get("WATCHTOWER_BRAIN_ADDR", "localhost:50052")
	agentClient, err := agent.NewClient(brainAddr, logger)
	if err != nil {
		logger.Warn("failed to connect to brain, running without diagnosis", zap.Error(err))
		agentClient = nil
	} else {
		defer agentClient.Close()
	}

	// 事件管理器（使用 internal/incident 包）
	incidentManager := incident.NewManager(hub, agentClient, autofixEngine, logger, database)

	alertHandler := gateway.NewHandler(logger, mockControlURL, func(alert *models.Alert) {
		incidentManager.OnAlert(alert)
	})
	defer alertHandler.Close()

	// Web 服务
	r := gin.Default()
	r.Use(func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
		c.Next()
	})
	alertHandler.RegisterRoutes(r)
	r.GET("/healthz", func(c *gin.Context) {
		brainStatus := "unavailable"
		if agentClient != nil {
			ctx, cancel := context.WithTimeout(c.Request.Context(), time.Second)
			defer cancel()
			if err := agentClient.HealthCheck(ctx); err == nil {
				brainStatus = "ok"
			}
		}
		status := http.StatusOK
		if brainStatus != "ok" {
			status = http.StatusServiceUnavailable
		}
		c.JSON(status, gin.H{"status": "ok", "gateway": "ok", "brain": brainStatus})
	})

	// WebSocket 路由
	upgrader := websocket.Upgrader{
		CheckOrigin: sameHostnameOrigin,
	}
	r.GET("/ws", func(c *gin.Context) {
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			logger.Error("websocket upgrade failed", zap.Error(err))
			return
		}
		hub.Register(conn)
	})

	// 事件列表 API
	r.GET("/api/v1/incidents", func(c *gin.Context) {
		items := incidentManager.ListIncidents()
		status, service := c.Query("status"), c.Query("service")
		filtered := make([]*models.Incident, 0, len(items))
		for _, item := range items {
			if status != "" && string(item.Status) != status {
				continue
			}
			if service != "" && !strings.Contains(strings.ToLower(item.Service), strings.ToLower(service)) {
				continue
			}
			filtered = append(filtered, item)
		}
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "100"))
		if page < 1 {
			page = 1
		}
		if pageSize < 1 {
			pageSize = 20
		}
		if pageSize > 500 {
			pageSize = 500
		}
		start := (page - 1) * pageSize
		if start > len(filtered) {
			start = len(filtered)
		}
		end := start + pageSize
		if end > len(filtered) {
			end = len(filtered)
		}
		c.JSON(http.StatusOK, gin.H{"incidents": filtered[start:end], "total": len(filtered), "page": page, "page_size": pageSize})
	})
	r.GET("/api/v1/incidents/:id", func(c *gin.Context) {
		id := c.Param("id")
		inc, ok := incidentManager.GetIncident(id)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "incident not found"})
			return
		}
		c.JSON(http.StatusOK, inc)
	})
	r.GET("/api/v1/incidents/:id/timeline", func(c *gin.Context) {
		if _, ok := incidentManager.GetIncident(c.Param("id")); !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "事件不存在"})
			return
		}
		entries, err := incidentManager.Timeline(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取时间轴失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"timeline": entries})
	})
	r.GET("/api/v1/incidents/:id/replay", func(c *gin.Context) {
		item, ok := incidentManager.GetIncident(c.Param("id"))
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "事件不存在"})
			return
		}
		entries, err := incidentManager.Timeline(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取回放数据失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"incident": item, "frames": entries, "frame_count": len(entries)})
	})

	// 静态文件服务（前端）
	r.Static("/assets", "./web/dist/assets")
	r.StaticFile("/", "./web/dist/index.html")

	port := config.Get("WATCHTOWER_PORT", "8080")
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: r,
	}

	// 启动服务
	go func() {
		logger.Info("Watchtower Gateway starting", zap.String("port", port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("server failed", zap.Error(err))
		}
	}()

	// 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("server shutdown error", zap.Error(err))
	}
}

func sameHostnameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	requestHost := strings.Split(r.Host, ":")[0]
	return strings.EqualFold(parsed.Hostname(), requestHost)
}
