package todo

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Create godoc
// @Summary Create todo
// @Tags todos
// @Accept json
// @Produce json
// @Param payload body CreateTodoInput true "todo payload"
// @Success 201 {object} Todo
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /api/v1/todos [post]
func (h *Handler) Create(c *gin.Context) {
	var input CreateTodoInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	todo, err := h.svc.Create(input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, todo)
}

// List godoc
// @Summary List todos
// @Tags todos
// @Produce json
// @Success 200 {array} Todo
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /api/v1/todos [get]
func (h *Handler) List(c *gin.Context) {
	todos, err := h.svc.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list todos"})
		return
	}
	c.JSON(http.StatusOK, todos)
}

// Get godoc
// @Summary Get todo by id
// @Tags todos
// @Produce json
// @Param id path int true "todo id"
// @Success 200 {object} Todo
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Security BearerAuth
// @Router /api/v1/todos/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	todo, err := h.svc.GetByID(id)
	if err != nil {
		if IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "todo not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get todo"})
		return
	}
	c.JSON(http.StatusOK, todo)
}

// Delete godoc
// @Summary Delete todo by id
// @Tags todos
// @Param id path int true "todo id"
// @Success 204
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /api/v1/todos/{id} [delete]
func (h *Handler) Delete(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.svc.DeleteByID(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete todo"})
		return
	}
	c.Status(http.StatusNoContent)
}

func parseID(raw string) (uint, error) {
	v, err := strconv.ParseUint(raw, 10, 64)
	return uint(v), err
}
