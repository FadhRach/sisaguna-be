package services

import (
	"errors"

	"github.com/FadhRach/Learn-Golang-React/project-management/models"
	"github.com/FadhRach/Learn-Golang-React/project-management/repositories"
	"github.com/FadhRach/Learn-Golang-React/project-management/utils"
	"github.com/google/uuid"
)

type UserService interface{
	Register(user *models.User) error
}

type userService struct {
	repo repositories.UserRepository
}

func NewUserService(repo repositories.UserRepository) UserService {
	return &userService{repo}
}

func (s *userService)Register(user *models.User) error {
	// cek email yang terdaftar
	existingUser, _ := s.repo.FindByEmail(user.Email)
	if existingUser.InternalID != 0 {
		return errors.New("email already registered")
	}
	// hashing dari password
	hased, err := utils.HashPassword(user.Password)
	if err != nil {
		return err
	}
	user.Password = hased
	// set role 
	user.Role = "user"
	// uuid baru karena create
	user.PublicID = uuid.New()

	// simpan user
	return s.repo.Create(user)
}