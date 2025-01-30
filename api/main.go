package main

import (
	"drizlink/api/router"
	"fmt"
	"log"
	"net/http"
)

func main() {
	// Setup routes
	router.SetupRoutes()

	// Start the API server
	fmt.Println("API server starting on :5000")
	if err := http.ListenAndServe(":5000", nil); err != nil {
		log.Fatal("API server failed to start:", err)
	}
}
