package session

import (
	"github.com/gofiber/utils/v2"
	"github.com/matchabrowser/syncserver/application/session/dto"
	"github.com/matchabrowser/syncserver/domain"
)

func (c *SessionApplication) Create(input dto.CreateSessionInput) (*dto.CreateSessionOutput, error) {
	logger := c.Logger.With().Str("component", "application.session.create").Logger()

	session := &domain.Session{
		Id:        utils.UUIDv4(),
		UserId:    input.UserId,
		UserAgent: input.UserAgent,
		IpAddress: input.IpAddress,
		ExpiresAt: input.ExpiresAt,
	}

	err := c.SessionPers.Create(session)
	if err != nil {
		logger.Error().Err(err).Str("session_id", session.Id).Str("user_id", session.UserId).Msg("failed to create session")
		return nil, err
	}

	return &dto.CreateSessionOutput{SessionId: session.Id}, nil
}

func (c *SessionApplication) DeleteExpired() error {
	// logger := c.Logger.With().Str("component", "application.session.delete_expired").Logger()

	return nil
}
