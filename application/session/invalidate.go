package session

import "github.com/matchabrowser/syncserver/application/session/dto"

func (c *SessionApplication) InvalidateSession(input dto.InvalidateSessionInput) error {
	logger := c.Logger.With().Str("component", "application.session.invalidate_session").Logger()

	err := c.SessionPers.DeleteById(input.SessionId)
	if err != nil {
		logger.Error().Err(err).Str("session_id", input.SessionId).Msg("failed to invalidate session")
		return err
	}

	return nil
}
