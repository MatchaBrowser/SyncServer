package auth

import (
	"fmt"

	"github.com/matchabrowser/syncserver/application/auth/dto"
	u "github.com/matchabrowser/syncserver/application/user/dto"
	"github.com/matchabrowser/syncserver/domain"
	"golang.org/x/crypto/bcrypt"
)

func (c *AuthApplication) Register(input dto.RegisterInput) error {
	logger := c.Logger.With().Str("component", "application.auth.register").Logger()

	// check if the email is already in use
	_, err := c.UserApplication.GetByEmail(u.GetByEmailInput{Email: input.Email})
	if err == nil {
		return fmt.Errorf("email is already in use")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		logger.Error().Err(err).Str("email", input.Email).Msg("failed to hash password")
		return fmt.Errorf("failed to hash password: %w", err)
	}

	user := domain.User{
		Username: input.Username,
		Email:    input.Email,
		Password: string(hashedPassword),
		Active:   true,
	}

	_, err = c.UserApplication.Create(u.CreateUserInput{User: user})
	if err != nil {
		logger.Error().Err(err).Str("email", input.Email).Msg("failed to create user")
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}
