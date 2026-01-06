// Package api provides REST API server implementation
package api

import (
	"github.com/gin-gonic/gin"
	"github.com/sanskarpan/db-backup/internal/api/middleware"
	"github.com/sanskarpan/db-backup/internal/backup"
	"github.com/sanskarpan/db-backup/internal/catalog"
	"github.com/sanskarpan/db-backup/internal/health"
	"github.com/sanskarpan/db-backup/internal/logger"
	"github.com/sanskarpan/db-backup/internal/restore"
	"github.com/sanskarpan/db-backup/internal/scheduler"
	"github.com/sanskarpan/db-backup/internal/security/ransomware"
)

// Server represents the API server
type Server struct {
	config        *Config
	backupEngine  *backup.Engine
	restoreEngine *restore.Engine
	scheduler     *scheduler.Scheduler
	healthChecker *health.Checker
	detector      *ransomware.Detector
	searchEngine  *catalog.SearchEngine
	logger        *logger.Logger
}

// Config holds API server configuration
type Config struct {
	Host          string
	Port          int
	LogLevel      string
	EnableCORS    bool
	EnableSwagger bool
	JWTSecret     string
	RateLimit     int
}

// NewServer creates a new API server
func NewServer(
	cfg *Config,
	backupEngine *backup.Engine,
	restoreEngine *restore.Engine,
	sched *scheduler.Scheduler,
	healthChecker *health.Checker,
	detector *ransomware.Detector,
	searchEngine *catalog.SearchEngine,
	log *logger.Logger,
) *Server {
	return &Server{
		config:        cfg,
		backupEngine:  backupEngine,
		restoreEngine: restoreEngine,
		scheduler:     sched,
		healthChecker: healthChecker,
		detector:      detector,
		searchEngine:  searchEngine,
		logger:        log,
	}
}

// SetupRoutes configures all API routes
func (s *Server) SetupRoutes(router *gin.Engine) {
	// Middleware - Order matters!

	// 1. Logging middleware (first to log all requests)
	router.Use(s.loggingMiddleware())

	// 2. Security headers (apply to all responses)
	router.Use(middleware.DefaultSecurityHeaders())

	// 3. CORS (if enabled)
	if s.config.EnableCORS {
		router.Use(s.corsMiddleware())
	}

	// 4. Request size limits (prevent DoS attacks)
	router.Use(middleware.DefaultMaxBodySize())

	// 5. CSRF protection (with exemptions for health/metrics endpoints)
	exemptPaths := []string{
		"/health",
		"/api/v1/health",
		"/api/v1/ready",
		"/api/v1/version",
		"/api/v1/metrics",
	}
	router.Use(middleware.CSRFProtectionWithExemptions(exemptPaths))

	// API v1 routes
	v1 := router.Group("/api/v1")
	{
		// Health and readiness
		v1.GET("/health", s.handleHealth)
		v1.GET("/ready", s.handleReady)
		v1.GET("/version", s.handleVersion)

		// Backup operations
		backups := v1.Group("/backups")
		{
			backups.POST("", s.handleCreateBackup)
			backups.GET("", s.handleListBackups)
			backups.GET("/:id", s.handleGetBackup)
			backups.DELETE("/:id", s.handleDeleteBackup)
			backups.POST("/:id/restore", s.handleRestoreBackup)
			backups.GET("/:id/download", s.handleDownloadBackup)
		}

		// Schedule management
		schedules := v1.Group("/schedules")
		{
			schedules.POST("", s.handleCreateSchedule)
			schedules.GET("", s.handleListSchedules)
			schedules.GET("/:id", s.handleGetSchedule)
			schedules.PUT("/:id", s.handleUpdateSchedule)
			schedules.DELETE("/:id", s.handleDeleteSchedule)
			schedules.POST("/:id/enable", s.handleEnableSchedule)
			schedules.POST("/:id/disable", s.handleDisableSchedule)
			schedules.POST("/:id/run", s.handleRunSchedule)
		}

		// Statistics and monitoring
		v1.GET("/stats", s.handleGetStats)
		v1.GET("/stats/storage", s.handleGetStorageStats)

		// Security endpoints
		security := v1.Group("/security")
		{
			// Ransomware detection
			security.POST("/scan/file", s.handleScanFile)
			security.POST("/scan/directory", s.handleScanDirectory)
			security.GET("/stats", s.handleGetSecurityStats)

			// Threat alerts
			security.GET("/alerts", s.handleListThreatAlerts)
			security.GET("/alerts/:id", s.handleGetThreatAlert)
			security.PUT("/alerts/:id", s.handleUpdateThreatAlert)

			// Immutable storage configuration
			security.GET("/storage/providers", s.handleListStorageProviders)
			security.GET("/storage/providers/:id", s.handleGetStorageProvider)
			security.PUT("/storage/providers/:id", s.handleUpdateStorageProvider)
		}

		// Catalog and search endpoints
		catalogRoutes := v1.Group("/catalog")
		{
			catalogRoutes.POST("/search", s.handleSearchCatalog)
			catalogRoutes.GET("/search", s.handleSearchCatalogSimple)
			catalogRoutes.GET("/suggest", s.handleSuggestCatalog)
			catalogRoutes.GET("/stats", s.handleGetCatalogStats)
			catalogRoutes.GET("/query-examples", s.handleQueryExamples)
		}
	}

	// Root endpoint
	router.GET("/", s.handleRoot)
}

// Response helpers
type ErrorResponse struct {
	Error   string                 `json:"error"`
	Message string                 `json:"message,omitempty"`
	Details map[string]interface{} `json:"details,omitempty"`
}

type SuccessResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Message string      `json:"message,omitempty"`
}

// Helper methods
func (s *Server) respondError(c *gin.Context, code int, err error, message string) {
	s.logger.Error("API error", err, map[string]interface{}{
		"message": message,
		"path":    c.Request.URL.Path,
		"method":  c.Request.Method,
	})

	c.JSON(code, ErrorResponse{
		Error:   err.Error(),
		Message: message,
	})
}

func (s *Server) respondSuccess(c *gin.Context, data interface{}) {
	c.JSON(200, SuccessResponse{
		Success: true,
		Data:    data,
	})
}

func (s *Server) respondSuccessWithMessage(c *gin.Context, message string, data interface{}) {
	c.JSON(200, SuccessResponse{
		Success: true,
		Message: message,
		Data:    data,
	})
}
