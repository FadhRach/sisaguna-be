package seed

import (
	"log"

	"github.com/FadhRach/Learn-Golang-React/project-management/config"
	"github.com/FadhRach/Learn-Golang-React/project-management/models"
	"github.com/FadhRach/Learn-Golang-React/project-management/utils"
	"github.com/google/uuid"
)

func SeedAdmin() {
	password, _ := utils.HashPassword("admin123")

	admin := models.User{
		Name:     "Superadmin",
		Email:    "admin@example.com",
		Password: password,
		Role:     "admin",
		PublicID: uuid.New(),
	}
	if err := config.DB.FirstOrCreate(&admin, models.User{Email: admin.Email}).Error; err != nil {
		log.Println("Failed too seed admin", err)
	} else {
		log.Println("Admin user seeded")
	}

}
