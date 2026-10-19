package user

import (
	"github.com/matchabrowser/syncserver/application/user/dto"
)

func (c *UserApplication) GetByEmail(input dto.GetByEmailInput) (*dto.GetByEmailOutput, error) {
	logger := c.Logger.With().Str("component", "application.user.get_by_email").Logger()

	user, err := c.UserPres.GetByEmail(input.Email)
	if err != nil {
		logger.Error().Err(err).Str("email", input.Email).Msg("failed to get user by email")
		return nil, err
	}
	return &dto.GetByEmailOutput{User: &user}, nil
}

func (c *UserApplication) GetByUserId(input dto.GetByUserIdInput) (*dto.GetByUserIdOutput, error) {
	logger := c.Logger.With().Str("component", "application.user.get_by_user_id").Logger()

	user, err := c.UserPres.GetById(input.UserId)
	if err != nil {
		logger.Error().Err(err).Str("user_id", input.UserId).Msg("failed to get user by id")
		return nil, err
	}
	return &dto.GetByUserIdOutput{User: &user}, nil
}
