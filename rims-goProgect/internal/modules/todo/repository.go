package todo

import "gorm.io/gorm"

type Repository interface {
	Create(todo *Todo) error
	List() ([]Todo, error)
	GetByID(id uint) (*Todo, error)
	DeleteByID(id uint) error
}

type GormRepository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *GormRepository {
	return &GormRepository{db: db}
}

func (r *GormRepository) Create(todo *Todo) error {
	return r.db.Create(todo).Error
}

func (r *GormRepository) List() ([]Todo, error) {
	var todos []Todo
	err := r.db.Order("id desc").Find(&todos).Error
	return todos, err
}

func (r *GormRepository) GetByID(id uint) (*Todo, error) {
	var todo Todo
	if err := r.db.First(&todo, id).Error; err != nil {
		return nil, err
	}
	return &todo, nil
}

func (r *GormRepository) DeleteByID(id uint) error {
	return r.db.Delete(&Todo{}, id).Error
}
