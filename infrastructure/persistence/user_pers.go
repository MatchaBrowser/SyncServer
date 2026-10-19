package persistence

import (
	"errors"

	"github.com/matchabrowser/syncserver/domain"
	"github.com/matchabrowser/syncserver/infrastructure/helpers/apperrors"
	"gorm.io/gorm"
)

type userPers struct {
	db *gorm.DB
}

func NewUserPers(db *gorm.DB) *userPers {
	return &userPers{db: db}
}

func (u *userPers) GetByUsername(username string) (domain.User, error) {
	var user domain.User
	err := u.db.Debug().Where("username = ?", username).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return user, apperrors.ErrUserNotFound
	}
	return user, err
}

func (u *userPers) GetByEmail(email string) (domain.User, error) {
	var user domain.User
	err := u.db.Debug().Where("email = ?", email).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return user, apperrors.ErrUserNotFound
	}
	return user, err
}

func (u *userPers) Create(user domain.User) (domain.User, error) {
	err := u.db.Create(&user).Error
	return user, err
}

func (u *userPers) GetById(id string) (domain.User, error) {
	var user domain.User
	err := u.db.Debug().Where("id = ?", id).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return user, apperrors.ErrUserNotFound
	}
	return user, err
}

func (u *userPers) Update(user *domain.User) error {
	return u.db.Model(user).Updates(map[string]any{
		"username":   user.Username,
		"avatar_url": user.AvatarUrl,
	}).Error
}

func (u *userPers) UpdatePassword(userId, hashedPassword string) error {
	return u.db.Model(&domain.User{}).Where("id = ?", userId).Update("password", hashedPassword).Error
}

// Admin methods

func (u *userPers) GetAll(limit, offset int) ([]domain.User, int64, error) {
	var users []domain.User
	var total int64

	// Count total
	if err := u.db.Model(&domain.User{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get paginated list
	query := u.db.Model(&domain.User{}).Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit).Offset(offset)
	}
	if err := query.Find(&users).Error; err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

func (u *userPers) UpdateActive(userId string, active bool) error {
	return u.db.Model(&domain.User{}).Where("id = ?", userId).Update("active", active).Error
}

func (u *userPers) Delete(userId string) error {
	return u.db.Where("id = ?", userId).Delete(&domain.User{}).Error
}
