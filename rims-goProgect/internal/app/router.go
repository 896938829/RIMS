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
	"rims-go/internal/middleware"
	"rims-go/internal/modules/user"
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

	// Services
	userSvc := user.NewUserService(userRepo, roleRepo, tokenSvc)
	roleSvc := user.NewRoleService(roleRepo)

	// Handlers
	userHandler := user.NewHandler(userSvc, roleSvc)

	// Public endpoints
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// API v1
	api := r.Group("/api/v1")
	user.RegisterRoutes(api, userHandler, authMw)

	return r
}
