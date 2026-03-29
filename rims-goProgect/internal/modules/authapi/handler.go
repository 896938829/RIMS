package authapi

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"rims-go/internal/auth"
)

type Handler struct {
	tokenSvc     *auth.TokenService
	demoUser     string
	demoPassword string
}

type LoginInput struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func NewHandler(tokenSvc *auth.TokenService, demoUser, demoPassword string) *Handler {
	return &Handler{
		tokenSvc:     tokenSvc,
		demoUser:     demoUser,
		demoPassword: demoPassword,
	}
}

// Login godoc
// @Summary Demo login
// @Description Returns JWT when username/password match demo credentials
// @Tags auth
// @Accept json
// @Produce json
// @Param payload body LoginInput true "login payload"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /api/v1/auth/login [post]
func (h *Handler) Login(c *gin.Context) {
	var input LoginInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if input.Username != h.demoUser || input.Password != h.demoPassword {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	token, err := h.tokenSvc.GenerateToken(1, input.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token})
}
