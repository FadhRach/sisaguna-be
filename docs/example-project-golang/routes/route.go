package routes

import (
	"log"

	"github.com/FadhRach/Learn-Golang-React/project-management/controllers"
	"github.com/gofiber/fiber/v2"
	"github.com/joho/godotenv"
)

func Setup(app *fiber.App, uc *controllers.UserController){
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error Loading .env File")
	}
	app.Post("/v1/auth/register", uc.Register)

}