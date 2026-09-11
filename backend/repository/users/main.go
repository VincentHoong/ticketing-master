package users

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

var ErrUserNotFound = errors.New("user not found")
var ErrUserEmailTaken = errors.New("email already registered")

type NewUser struct {
	Name         string
	Email        string
	PasswordHash string
}

type UserItem struct {
	Id        string     `db:"id" json:"id"`
	Name      string     `db:"name" json:"name"`
	Email     string     `db:"email" json:"email"`
	CreatedAt *time.Time `db:"created_at" json:"createdAt"`
	UpdatedAt *time.Time `db:"updated_at" json:"updatedAt"`
}

type IUserRepository interface {
	GetUser(ctx context.Context, id string) (*UserItem, error)
	CreateUser(ctx context.Context, name string, email string, passwordHash string) (*UserItem, error)
	CreateUsers(ctx context.Context, users []NewUser) ([]string, error)
	ListUsers(ctx context.Context, limit uint64) ([]UserItem, error)
	CountUsers(ctx context.Context) (int64, error)
}

type UserRepository struct {
	PostgresRepository    *PostgresUserRepository
	RemoteCacheRepository *RemoteCacheUserRepository
}

func NewUserRepository(dbpool *pgxpool.Pool, rdb *redis.Client) IUserRepository {
	return &UserRepository{
		PostgresRepository:    newPostgresUserRepository(dbpool),
		RemoteCacheRepository: newRemoteCacheUserRepository(rdb),
	}
}

func (e *UserRepository) GetUser(ctx context.Context, id string) (*UserItem, error) {
	if cached, err := e.RemoteCacheRepository.GetUser(ctx, id); err == nil {
		return cached, nil
	}

	userItem, err := e.PostgresRepository.GetUser(ctx, id)
	if err != nil {
		return nil, err
	}

	if err := e.RemoteCacheRepository.SetUser(ctx, userItem); err != nil {
		log.Printf("get user: cache set: %v", err)
	}

	return userItem, nil
}

func (e *UserRepository) CreateUser(ctx context.Context, name string, email string, passwordHash string) (*UserItem, error) {
	return e.PostgresRepository.CreateUser(ctx, name, email, passwordHash)
}

func (e *UserRepository) CreateUsers(ctx context.Context, users []NewUser) ([]string, error) {
	return e.PostgresRepository.CreateUsers(ctx, users)
}

func (e *UserRepository) ListUsers(ctx context.Context, limit uint64) ([]UserItem, error) {
	return e.PostgresRepository.ListUsers(ctx, limit)
}

func (e *UserRepository) CountUsers(ctx context.Context) (int64, error) {
	return e.PostgresRepository.CountUsers(ctx)
}
