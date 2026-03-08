package users

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type UseCases struct {
	repo Repository
}

func NewUseCases(repo Repository) *UseCases {
	return &UseCases{repo: repo}
}

type RegisterUserInput struct {
	TenantID uuid.UUID
	Email    string
	Password string
	Role     string
}

func (uc *UseCases) Register(ctx context.Context, input RegisterUserInput) (*User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	user := &User{
		ID:           id,
		TenantID:     input.TenantID,
		Email:        input.Email,
		PasswordHash: string(hash),
		Role:         input.Role,
	}

	if err := uc.repo.Save(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

func (uc *UseCases) Get(ctx context.Context, id uuid.UUID) (*User, error) {
	return uc.repo.FindByID(ctx, id)
}

func (uc *UseCases) FindByEmail(ctx context.Context, email string) (*User, error) {
	return uc.repo.FindByEmail(ctx, email)
}
