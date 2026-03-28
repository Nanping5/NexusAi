package dao

import (
	mmysql "NexusAi/common/mysql"
	"NexusAi/model"
	"NexusAi/pkg/utils"
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// 定义明确的错误类型
var (
	ErrUserNotFound    = errors.New("user not found")
	ErrInvalidPassword = errors.New("invalid password")
)

// UserDAO 全局用户 DAO 实例
var UserDAO = &userDAO{}

type userDAO struct {
}

// Register 注册新用户
func (dao *userDAO) Register(ctx context.Context, email, password string) (*model.User, error) {
	db, err := mmysql.NewDbClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("get db client failed: %w", err)
	}

	// 生成唯一的 6 位 UserID
	userID, err := dao.generateUniqueUserID(db)
	if err != nil {
		return nil, fmt.Errorf("generate user id failed: %w", err)
	}

	// 生成默认昵称
	nickname := utils.GenerateNickname()

	user := model.User{
		UserID:   userID,
		Email:    email,
		Password: password,
		Nickname: nickname,
	}
	if err := user.EncryptPassword(); err != nil {
		return nil, fmt.Errorf("encrypt password failed: %w", err)
	}
	if err := db.Create(&user).Error; err != nil {
		return nil, fmt.Errorf("create user failed: %w", err)
	}
	return &user, nil
}

// generateUniqueUserID 生成唯一的用户 ID（U + 6位数字）
func (dao *userDAO) generateUniqueUserID(db *gorm.DB) (string, error) {
	const maxAttempts = 10
	for i := 0; i < maxAttempts; i++ {
		userID := utils.GenerateUserID() // 生成 U + 6位数字

		// 检查是否已存在
		var count int64
		if err := db.Model(&model.User{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
			return "", fmt.Errorf("check user id existence failed: %w", err)
		}
		if count == 0 {
			return userID, nil
		}
	}
	return "", fmt.Errorf("failed to generate unique user id after %d attempts", maxAttempts)
}

// IsEmailExist 检查邮箱是否已存在
func (dao *userDAO) IsEmailExist(ctx context.Context, email string) (bool, error) {
	db, err := mmysql.NewDbClient(ctx)
	if err != nil {
		return false, fmt.Errorf("get db client failed: %w", err)
	}
	var count int64
	if err := db.Model(&model.User{}).Where("email = ?", email).Count(&count).Error; err != nil {
		return false, fmt.Errorf("check email existence failed: %w", err)
	}
	return count > 0, nil
}

// Login 邮箱登录
func (dao *userDAO) Login(ctx context.Context, email, password string) (*model.User, error) {
	db, err := mmysql.NewDbClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("get db client failed: %w", err)
	}
	user := model.User{}
	if err := db.Where("email = ?", email).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("query user failed: %w", err)
	}
	if !user.CheckPassword(password) {
		return nil, ErrInvalidPassword
	}
	return &user, nil
}

// GetUserInfo 获取用户信息
func (dao *userDAO) GetUserInfo(ctx context.Context, userID string) (*model.User, error) {
	db, err := mmysql.NewDbClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("get db client failed: %w", err)
	}
	user := model.User{}
	if err := db.Where("user_id = ?", userID).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("query user failed: %w", err)
	}
	return &user, nil
}

// UpdateNickname 更新用户昵称
func (dao *userDAO) UpdateNickname(ctx context.Context, userID, nickname string) error {
	db, err := mmysql.NewDbClient(ctx)
	if err != nil {
		return fmt.Errorf("get db client failed: %w", err)
	}
	if err := db.Model(&model.User{}).Where("user_id = ?", userID).Update("nickname", nickname).Error; err != nil {
		return fmt.Errorf("update nickname failed: %w", err)
	}
	return nil
}
