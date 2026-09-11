package users

import (
	"context"
	"errors"
	"ticketing-master/repository"
	"ticketing-master/repository/users"

	"golang.org/x/crypto/bcrypt"
)

const defaultUserListLimit = 100

var ErrInvalidUser = errors.New("user name and email are required")

type CreateUserRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type IUserService interface {
	GetUser(ctx context.Context, id string) (*users.UserItem, error)
	CreateUser(ctx context.Context, request *CreateUserRequest) (*users.UserItem, error)
	ListUsers(ctx context.Context, limit uint64) ([]users.UserItem, error)
}

type UserService struct {
	Repositories *repository.Repositories
}

func NewUserService(repositories *repository.Repositories) IUserService {
	return &UserService{Repositories: repositories}
}

func (s *UserService) GetUser(ctx context.Context, id string) (*users.UserItem, error) {
	return s.Repositories.UserRepository.GetUser(ctx, id)
}

func (s *UserService) CreateUser(ctx context.Context, request *CreateUserRequest) (*users.UserItem, error) {
	if request.Name == "" || request.Email == "" {
		return nil, ErrInvalidUser
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(request.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	return s.Repositories.UserRepository.CreateUser(ctx, request.Name, request.Email, string(passwordHash))
}

func (s *UserService) ListUsers(ctx context.Context, limit uint64) ([]users.UserItem, error) {
	if limit == 0 || limit > defaultUserListLimit {
		limit = defaultUserListLimit
	}

	return s.Repositories.UserRepository.ListUsers(ctx, limit)
}
