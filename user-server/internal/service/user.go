package service

import (
	"context"
	"errors"
	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/pkg/utils"
	"marketing/internal/pkg/utils/bcrypt"
	_type "marketing/internal/pkg/utils/type"
	"marketing/internal/repository"
	"strconv"
)

type UserService interface {
	RegisterUser(ctx context.Context, req *dto.CreateUserRequest) (*dto.UserResponse, error)
	GetUser(ctx context.Context, id string) (*dto.UserResponse, error)
	GetUserByUsername(ctx context.Context, username string) (*dto.UserResponse, error)
	GetUserList(ctx context.Context, page int, limit int) (*dto.GetUserListResponse, error)
	DeleteUser(ctx context.Context, id string) error
	UpdateUser(ctx context.Context, id string, req *dto.UpdateUserRequest) (*dto.UserResponse, error)
	UpdatePassword(ctx context.Context, id string, req *dto.UpdatePasswordRequest) error
	Login(ctx context.Context, req *dto.LoginRequest) (*dto.LoginResponse, error)
	InitUser(ctx context.Context, accountID string, tgID int64, FirstName string, LastName string, UserName string) (string, error)
}

type userService struct {
	userRepo repository.UserRepository
	jwtUtils *utils.JWTUtils
}

func NewUserService() UserService {
	return &userService{
		userRepo: repository.NewUserRepository(),
		jwtUtils: utils.NewJWTUtils(utils.DefaultJWTConfig),
	}
}

func (s *userService) RegisterUser(ctx context.Context, req *dto.CreateUserRequest) (*dto.UserResponse, error) {
	// 检查用户名是否已存在
	exists, err := s.userRepo.UsernameExists(ctx, req.Username, "")
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, errors.New("用户名已存在")
	}

	// 检查邮箱是否已存在
	if req.Email != "" {
		exists, err = s.userRepo.EmailExists(ctx, req.Email, "")
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, errors.New("邮箱已存在")
		}
	}

	user := &model.User{
		Username: req.Username,
		Password: req.Password, // BeforeCreate 钩子会自动哈希密码
		Email:    req.Email,
		RealName: req.RealName,
		Phone:    req.Phone,
		Avatar:   req.Avatar,
		Role:     req.Role,
		Status:   1, // 默认激活状态
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, err
	}

	return &dto.UserResponse{
		ID:       user.ID,
		Username: user.Username,
		Email:    user.Email,
		RealName: user.RealName,
		Phone:    user.Phone,
		Avatar:   user.Avatar,
		Role:     user.Role,
		Status:   _type.UserStatusType(user.Status),
	}, nil
}

func (s *userService) GetUser(ctx context.Context, id string) (*dto.UserResponse, error) {
	user, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	return &dto.UserResponse{
		ID:         user.ID,
		TgID:       user.TgID,
		Username:   user.Username,
		Email:      user.Email,
		RealName:   user.RealName,
		Phone:      user.Phone,
		Avatar:     user.Avatar,
		Role:       user.Role,
		Status:     _type.UserStatusType(user.Status),
		CreateTime: user.CreateTime,
		UpdateTime: user.UpdateTime,
	}, nil
}

func (s *userService) GetUserByUsername(ctx context.Context, username string) (*dto.UserResponse, error) {
	user, err := s.userRepo.GetByUsername(ctx, username)
	if err != nil {
		return nil, err
	}

	return &dto.UserResponse{
		ID:         user.ID,
		TgID:       user.TgID,
		Username:   user.Username,
		Email:      user.Email,
		RealName:   user.RealName,
		Phone:      user.Phone,
		Avatar:     user.Avatar,
		Role:       user.Role,
		Status:     _type.UserStatusType(user.Status),
		CreateTime: user.CreateTime,
		UpdateTime: user.UpdateTime,
	}, nil
}

func (s *userService) GetUserList(ctx context.Context, page int, limit int) (*dto.GetUserListResponse, error) {
	users, total, err := s.userRepo.GetUserList(ctx, page, limit)
	if err != nil {
		return nil, err
	}

	var userResponses []*dto.UserResponse
	for _, user := range users {
		userResponses = append(userResponses, &dto.UserResponse{
			ID:         user.ID,
			TgID:       user.TgID,
			Username:   user.Username,
			Email:      user.Email,
			RealName:   user.RealName,
			Phone:      user.Phone,
			Avatar:     user.Avatar,
			Role:       user.Role,
			Status:     _type.UserStatusType(user.Status),
			CreateTime: user.CreateTime,
			UpdateTime: user.UpdateTime,
		})
	}

	return &dto.GetUserListResponse{
		Users: userResponses,
		Total: total,
	}, nil
}

func (s *userService) DeleteUser(ctx context.Context, id string) error {
	return s.userRepo.Delete(ctx, id)
}

func (s *userService) UpdateUser(ctx context.Context, id string, req *dto.UpdateUserRequest) (*dto.UserResponse, error) {
	user, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// 检查用户名是否已存在（排除当前用户）
	if req.Username != "" && req.Username != user.Username {
		exists, err := s.userRepo.UsernameExists(ctx, req.Username, id)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, errors.New("用户名已存在")
		}
		user.Username = req.Username
	}

	// 检查邮箱是否已存在（排除当前用户）
	if req.Email != "" && req.Email != user.Email {
		exists, err := s.userRepo.EmailExists(ctx, req.Email, id)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, errors.New("邮箱已存在")
		}
		user.Email = req.Email
	}

	// 更新其他字段
	if req.RealName != "" {
		user.RealName = req.RealName
	}
	if req.Phone != "" {
		user.Phone = req.Phone
	}
	if req.Avatar != "" {
		user.Avatar = req.Avatar
	}
	if req.Role != "" {
		user.Role = req.Role
	}
	if req.Status != nil {
		user.Status = _type.UserStatusType(*req.Status)
	}

	if err := s.userRepo.Update(ctx, user); err != nil {
		return nil, err
	}

	return &dto.UserResponse{
		ID:         user.ID,
		TgID:       user.TgID,
		Username:   user.Username,
		Email:      user.Email,
		RealName:   user.RealName,
		Phone:      user.Phone,
		Avatar:     user.Avatar,
		Role:       user.Role,
		Status:     _type.UserStatusType(user.Status),
		CreateTime: user.CreateTime,
		UpdateTime: user.UpdateTime,
	}, nil
}

func (s *userService) UpdatePassword(ctx context.Context, id string, req *dto.UpdatePasswordRequest) error {
	user, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	// 验证旧密码
	if err := bcrypt.CheckPassword(user.Password, req.OldPassword); err != nil {
		return errors.New("原密码不正确")
	}

	// 加密新密码
	hashedPassword, err := bcrypt.HashPassword(req.NewPassword)
	if err != nil {
		return err
	}

	return s.userRepo.UpdatePassword(ctx, id, hashedPassword)
}

func (s *userService) Login(ctx context.Context, req *dto.LoginRequest) (*dto.LoginResponse, error) {
	user, err := s.userRepo.GetByUsername(ctx, req.Username)
	if err != nil {
		return nil, errors.New("用户名或密码错误")
	}

	// 验证密码
	if err := bcrypt.CheckPassword(user.Password, req.Password); err != nil {
		return nil, errors.New("用户名或密码错误")
	}

	// 检查用户状态
	if user.Status != 1 {
		return nil, errors.New("账户已被禁用")
	}

	// 修复：使用真正的 JWT 工具生成 token（不再用假"jwt_token_"字符串）
	jwtUtils := utils.NewJWTUtils(utils.DefaultJWTConfig)
	// user.ID 是 string（uuid），转换为 uint（JWT 内部用 uint 表示 user_id）
	var userIDUint uint
	if v, err := strconv.ParseUint(user.ID, 10, 64); err == nil {
		userIDUint = uint(v)
	}
	token, err := jwtUtils.GenerateToken(userIDUint, user.Username, user.Role)
	if err != nil {
		return nil, err
	}

	return &dto.LoginResponse{
		Token: token,
		User: dto.UserResponse{
			ID:         user.ID,
			TgID:       user.TgID,
			Username:   user.Username,
			Email:      user.Email,
			RealName:   user.RealName,
			Phone:      user.Phone,
			Avatar:     user.Avatar,
			Role:       user.Role,
			Status:     _type.UserStatusType(user.Status),
			CreateTime: user.CreateTime,
			UpdateTime: user.UpdateTime,
		},
	}, nil
}

func (s *userService) InitUser(ctx context.Context, accountID string, tgID int64, FirstName string, LastName string, UserName string) (string, error) {
	userId, exists := s.userRepo.UserIsExist(ctx, accountID, tgID, FirstName, LastName, UserName)
	if exists {
		return userId, nil
	}
	user := model.User{
		AccountID: accountID,
		TgID:      tgID,
		FirstName: FirstName,
		LastName:  LastName,
		UserName:  UserName,
	}
	if err := s.userRepo.Create(ctx, &user); err != nil {
		return "", err
	}
	return user.ID, nil
}
