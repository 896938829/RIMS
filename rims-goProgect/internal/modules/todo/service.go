package todo

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

type CreateTodoInput struct {
	Title string `json:"title" binding:"required"`
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(input CreateTodoInput) (*Todo, error) {
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return nil, fmt.Errorf("title is required")
	}
	todo := &Todo{Title: title, Done: false}
	if s.repo == nil {
		return todo, nil
	}
	if err := s.repo.Create(todo); err != nil {
		return nil, err
	}
	return todo, nil
}

func (s *Service) List() ([]Todo, error) {
	return s.repo.List()
}

func (s *Service) GetByID(id uint) (*Todo, error) {
	return s.repo.GetByID(id)
}

func (s *Service) DeleteByID(id uint) error {
	return s.repo.DeleteByID(id)
}

func IsNotFound(err error) bool {
	return err != nil && err == gorm.ErrRecordNotFound
}
