// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package app

import (
	"net/http"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/gorm"

	_ "rims-go/docs"
	"rims-go/internal/auth"
	"rims-go/internal/config"
	"rims-go/internal/db"
	"rims-go/internal/middleware"
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

	// Repositories
	userRepo := user.NewUserRepository(gormDB)
	roleRepo := user.NewRoleRepository(gormDB)
	warehouseRepo := warehouse.NewWarehouseRepository(gormDB)
	userWarehouseRepo := warehouse.NewUserWarehouseRepository(gormDB)

	// Services
	userSvc := user.NewUserService(userRepo, roleRepo, tokenSvc)
	roleSvc := user.NewRoleService(roleRepo)
	warehouseSvc := warehouse.NewWarehouseService(warehouseRepo, userWarehouseRepo, db.NewTxRunner(gormDB))

	// Handlers
	userHandler := user.NewHandler(userSvc, roleSvc)
	warehouseHandler := warehouse.NewHandler(warehouseSvc)

	// Warehouse scope middleware (for future modules: product, document, etc.)
	_ = middleware.WarehouseScope(userWarehouseRepo)

	// Public endpoints
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// API v1
	api := r.Group("/api/v1")
	user.RegisterRoutes(api, userHandler, authMw)
	warehouse.RegisterRoutes(api, warehouseHandler, authMw)

	return r
}
