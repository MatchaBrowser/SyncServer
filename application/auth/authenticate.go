package auth

import (
	"fmt"
	"time"

	"github.com/matchabrowser/syncserver/application/auth/dto"
	s "github.com/matchabrowser/syncserver/application/session/dto"
	u "github.com/matchabrowser/syncserver/application/user/dto"
	"github.com/matchabrowser/syncserver/infrastructure/helpers/apperrors"
	"github.com/matchabrowser/syncserver/infrastructure/helpers/tokenutil"
	"golang.org/x/crypto/bcrypt"
)

func (c *AuthApplication) Authenticate(input dto.AuthenticateInput) (*dto.AuthenticateOutput, error) {
	logger := c.Logger.With().Str("component", "application.auth.authenticate").Logger()

	resp, err := c.UserApplication.GetByEmail(u.GetByEmailInput{Email: input.Email})
	if err != nil {
		return nil, fmt.Errorf("failed to get user by email: %w", err)
	}

	if !resp.User.Active {
		logger.Warn().Str("email", input.Email).Msg("attempt to authenticate inactive user")
		return nil, apperrors.ErrUserNotActive
	}

	err = bcrypt.CompareHashAndPassword([]byte(resp.User.Password), []byte(input.Password))
	if err != nil {
		logger.Warn().Str("email", input.Email).Msg("invalid password attempt")
		return nil, apperrors.ErrInvalidCredentials
	}

	sessionResult, err := c.SessionApplication.Create(s.CreateSessionInput{
		UserId:    resp.User.Id,
		UserAgent: input.Context.Get("User-Agent"),
		IpAddress: input.Context.IP(),
		ExpiresAt: time.Now().Add(time.Minute * time.Duration(c.Config.Session.ExpirationMinutes)),
	})
	if err != nil {
		logger.Error().Err(err).Str("user_id", resp.User.Id).Msg("failed to create session")
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	accessToken, err := tokenutil.CreateAccessToken(resp.User.Id, sessionResult.SessionId, c.Config)
	if err != nil {
		logger.Error().Err(err).Str("user_id", resp.User.Id).Str("session_id", sessionResult.SessionId).Msg("failed to create access token")
		return nil, fmt.Errorf("failed to create access token: %w", err)
	}

	return &dto.AuthenticateOutput{
		Token: accessToken,
	}, nil
}
