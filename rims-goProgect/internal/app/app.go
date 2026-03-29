// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package app

import (
	"fmt"
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
	"rims-go/internal/modules/authapi"
	"rims-go/internal/modules/todo"
)

// @title rims-go API
// @version 1.0
// @description rims-go backend API docs
// @BasePath /
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization

// Run boots the HTTP server with environment-based configuration.
func Run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	gormDB, err := db.New(cfg)
	if err != nil {
		return fmt.Errorf("init db: %w", err)
	}
	if cfg.DBAutoMigrate {
		if err := gormDB.AutoMigrate(&todo.Todo{}); err != nil {
			return fmt.Errorf("auto migrate: %w", err)
		}
	}

	addr := fmt.Sprintf(":%s", cfg.AppPort)
	server := &http.Server{
		Addr:    addr,
		Handler: buildRouter(cfg, gormDB),
	}

	return server.ListenAndServe()
}

func buildRouter(cfg config.Config, gormDB *gorm.DB) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())

	tokenSvc := auth.NewTokenService(cfg.JWTSecret, cfg.JWTExpireHours)
	authHandler := authapi.NewHandler(tokenSvc, cfg.DemoUser, cfg.DemoPassword)
	todoRepo := todo.NewRepository(gormDB)
	todoSvc := todo.NewService(todoRepo)
	todoHandler := todo.NewHandler(todoSvc)

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	api := r.Group("/api/v1")
	api.POST("/auth/login", authHandler.Login)

	protected := api.Group("")
	protected.Use(middleware.JWTAuth(tokenSvc))
	protected.POST("/todos", todoHandler.Create)
	protected.GET("/todos", todoHandler.List)
	protected.GET("/todos/:id", todoHandler.Get)
	protected.DELETE("/todos/:id", todoHandler.Delete)

	return r
}
