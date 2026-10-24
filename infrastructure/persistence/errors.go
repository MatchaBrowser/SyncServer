package persistence

import (
	"errors"

	"github.com/matchabrowser/syncserver/domain"

	"gorm.io/gorm"
)

// translate maps the errors of GORM to those of the domain. The database is
// opened with TranslateError, so a unique violation is gorm.ErrDuplicatedKey.
func translate(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.ErrNotFound
	case errors.Is(err, gorm.ErrDuplicatedKey):
		return domain.ErrDuplicate
	default:
		return err
	}
}
