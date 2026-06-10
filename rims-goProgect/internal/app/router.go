// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package app

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/gorm"

	_ "rims-go/docs"
	"rims-go/internal/auth"
	"rims-go/internal/config"
	"rims-go/internal/db"
	"rims-go/internal/idempotency"
	"rims-go/internal/middleware"
	"rims-go/internal/modules/audit"
	"rims-go/internal/modules/document"
	"rims-go/internal/modules/file"
	"rims-go/internal/modules/product"
	"rims-go/internal/modules/report"
	"rims-go/internal/modules/user"
	"rims-go/internal/modules/warehouse"
)

// buildRouter creates the Gin engine with all middleware and routes.
func buildRouter(cfg config.Config, gormDB *gorm.DB) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.RequestID())
	r.Use(middleware.Logger())
	r.Use(middleware.CORS(cfg.CORSOrigins))

	// Services
	tokenSvc := auth.NewTokenService(cfg.JWTSecret, cfg.JWTExpireHours)
	authMw := middleware.JWTAuth(tokenSvc)

	// Audit module — built first so it can be injected into downstream services
	// and handlers that need atomic or best-effort audit writes.
	auditRepo := audit.NewAuditRepository(gormDB)
	auditSvc := audit.NewAuditService(auditRepo)
	auditHandler := audit.NewHandler(auditSvc)

	// Repositories
	userRepo := user.NewUserRepository(gormDB)
	roleRepo := user.NewRoleRepository(gormDB)
	warehouseRepo := warehouse.NewWarehouseRepository(gormDB)
	userWarehouseRepo := warehouse.NewUserWarehouseRepository(gormDB)
	permMw := func(code string) gin.HandlerFunc {
		return middleware.Permission(roleRepo, code)
	}

	// Services
	userSvc := user.NewUserService(userRepo, roleRepo, tokenSvc)
	roleSvc := user.NewRoleService(roleRepo)
	warehouseSvc := warehouse.NewWarehouseService(warehouseRepo, userWarehouseRepo, db.NewTxRunner(gormDB))

	// Handlers
	userHandler := user.NewHandler(userSvc, roleSvc, auditSvc)
	warehouseHandler := warehouse.NewHandler(warehouseSvc, auditSvc)

	// Warehouse scope middleware
	whScope := middleware.WarehouseScope(userWarehouseRepo)

	// Idempotency middleware for selected unsafe write endpoints.
	idemRepo := idempotency.NewRepository(gormDB)
	idemSvc := idempotency.NewService(idemRepo, idempotencyTTLFromConfig(cfg))
	idemMw := middleware.Idempotency(idemSvc, cfg.MaxUploadMB)

	// Product module
	productRepo := product.NewProductRepository(gormDB)
	inventoryRepo := product.NewInventoryRepository(gormDB)
	nonStdRepo := product.NewNonStdInventoryRepository(gormDB)
	productSvc := product.NewProductService(productRepo, inventoryRepo, nonStdRepo, db.NewTxRunner(gormDB))
	productHandler := product.NewHandler(productSvc, auditSvc)

	// Document module
	docRepo := document.NewDocumentRepository(gormDB)
	docLineRepo := document.NewDocumentLineRepository(gormDB)
	txnRepo := document.NewInventoryTransactionRepository(gormDB)
	docSvc := document.NewDocumentService(
		docRepo, docLineRepo, txnRepo,
		inventoryRepo, nonStdRepo, productRepo,
		db.NewTxRunner(gormDB),
		auditSvc,
	)
	docHandler := document.NewHandler(docSvc)

	// Report module
	reportRepo := report.NewReportRepository(gormDB)
	reportSvc := report.NewReportService(reportRepo)
	reportHandler := report.NewHandler(reportSvc)

	// File module — local storage for v1; see README TODO for object storage.
	const filePublicPrefix = "/uploads"
	uploadDir := cfg.UploadDir
	if uploadDir == "" {
		uploadDir = "./uploads"
	}
	localStorage, err := file.NewLocalStorage(uploadDir, filePublicPrefix)
	if err != nil {
		log.Panicf("init file storage: %v", err)
	}
	fileRepo := file.NewFileRepository(gormDB)
	fileACL := fileAccessChecker{docRepo: docRepo, whRepo: userWarehouseRepo}
	fileSvc := file.NewFileService(
		fileRepo, localStorage,
		cfg.MaxUploadMB, cfg.AllowedExts,
		"/api/v1/files/%d/download",
		fileACL,
	)
	fileHandler := file.NewHandler(fileSvc, auditSvc)

	// Serve public file objects from the local uploads directory.
	r.Static(filePublicPrefix, localStorage.BaseDir())

	// Public endpoints
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// API v1
	api := r.Group("/api/v1")
	user.RegisterRoutes(api, userHandler, authMw, permMw)
	warehouse.RegisterRoutes(api, warehouseHandler, authMw, permMw)
	product.RegisterRoutes(api, productHandler, authMw, whScope, idemMw, permMw)
	document.RegisterRoutes(api, docHandler, authMw, whScope, idemMw)
	report.RegisterRoutes(api, reportHandler, authMw, whScope)
	file.RegisterRoutes(api, fileHandler, authMw, idemMw)
	audit.RegisterRoutes(api, auditHandler, authMw, permMw)

	return r
}

func idempotencyTTLFromConfig(cfg config.Config) time.Duration {
	return time.Duration(cfg.IdempotencyKeyTTLHours) * time.Hour
}
