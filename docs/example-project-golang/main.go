package main

import (
	"log"

	"github.com/FadhRach/Learn-Golang-React/project-management/config"
	"github.com/FadhRach/Learn-Golang-React/project-management/controllers"
	"github.com/FadhRach/Learn-Golang-React/project-management/database/seed"
	"github.com/FadhRach/Learn-Golang-React/project-management/repositories"
	"github.com/FadhRach/Learn-Golang-React/project-management/routes"
	"github.com/FadhRach/Learn-Golang-React/project-management/services"
	"github.com/gofiber/fiber/v2"
)

func main() {
	config.LoadEnv()
	config.ConnectDB()

	seed.SeedAdmin()

	app := fiber.New()

	userRepo := repositories.NewUserRepository()
	userService := services.NewUserService(userRepo)
	userController := controllers.NewUserController(userService)

	routes.Setup(app, userController)

	port := config.AppConfig.AppPort
	log.Println("Server is running on port", port)
	log.Fatal(app.Listen(":" + port))

}
