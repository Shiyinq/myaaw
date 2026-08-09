package repository

import (
	"database/sql"
	"fmt"
	"myaaw/internal/config"
	"myaaw/internal/services/bot/model"
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"
)

type UserRepository interface {
	CreateUser(user *model.User) (*model.User, error)
	GetUserById(userId int) (*model.User, error)
	UpdateModel(userId int, model string) error
	UpdateProvider(userId int, provider string) error
}

type UserRepositoryImpl struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) UserRepository {
	return &UserRepositoryImpl{db: db}
}

func (r *UserRepositoryImpl) GetUserById(userId int) (*model.User, error) {
	var user model.User
	query := `SELECT id, user_id, name, provider, model, role, created_at, updated_at FROM users WHERE user_id = ?`
	err := r.db.QueryRow(query, userId).Scan(
		&user.Id, &user.UserId, &user.Name, &user.Provider, &user.Model, &user.Role, &user.CreatedAt, &user.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return &user, nil
}

func (r *UserRepositoryImpl) CreateUser(user *model.User) (*model.User, error) {
	role := "user"
	userIDStr := strconv.Itoa(user.UserId)
	if slices.Contains(config.OwnerIDs, userIDStr) {
		role = "owner"
	}

	user.Id = uuid.New().String()
	user.Role = role
	user.CreatedAt = time.Now()
	user.UpdatedAt = time.Now()

	query := `INSERT INTO users (id, user_id, name, provider, model, role, created_at, updated_at) 
	          VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := r.db.Exec(query,
		user.Id, user.UserId, user.Name, user.Provider, user.Model, user.Role, user.CreatedAt, user.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	return r.GetUserById(user.UserId)
}

func (r *UserRepositoryImpl) UpdateModel(userId int, modelName string) error {
	query := `UPDATE users SET model = ?, updated_at = ? WHERE user_id = ?`
	_, err := r.db.Exec(query, modelName, time.Now(), userId)
	if err != nil {
		return fmt.Errorf("failed to update model: %w", err)
	}
	return nil
}

func (r *UserRepositoryImpl) UpdateProvider(userId int, provider string) error {
	query := `UPDATE users SET provider = ?, updated_at = ? WHERE user_id = ?`
	_, err := r.db.Exec(query, provider, time.Now(), userId)
	if err != nil {
		return fmt.Errorf("failed to update provider: %w", err)
	}
	return nil
}
