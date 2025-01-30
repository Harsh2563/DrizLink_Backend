package router

import (
	"drizlink/api/controllers"
	"drizlink/api/middleware"
	"net/http"
)

func SetupRoutes() {
	// Register routes with middleware
	http.HandleFunc("/api/start", middleware.CorsMiddleware(controllers.StartServer))
}
