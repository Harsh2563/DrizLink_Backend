package controllers

import (
	server "drizlink/server/cmd"
	"encoding/json"
	"net/http"
)

type IPRequest struct {
	IP string `json:"ip"`
}

type IDRequest struct {
	ID string `json:"id"`
}

type Response struct {
	Message string `json:"message"`
	Error   string `json:"error,omitempty"`
}

func StartServer(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(Response{
			Error: "Only POST method is allowed",
		})
		return
	}

	var req IPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(Response{
			Error: "Invalid request body: " + err.Error(),
		})
		return
	}

	if req.IP == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(Response{
			Error: "IP address is required",
		})
		return
	}

	// Start the websocket server
	err := server.StartServer(req.IP)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(Response{
			Error: "Failed to start WebSocket server: " + err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(Response{
		Message: "WebSocket server started on " + req.IP + ":8080",
	})
}

func GetUsers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(Response{
			Error: "Only POST method is allowed",
		})
		return
	}

	var req IPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(Response{
			Error: "Invalid request body: " + err.Error(),
		})
		return
	}

	if req.IP == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(Response{
			Error: "IP address is required",
		})
		return
	}

	// Get a list of all connected users
	users, err := server.GetConnectedUsers(req.IP)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(Response{
			Error: "Failed to get connected users: " + err.Error(),
		})
		return
	}
	json.NewEncoder(w).Encode(users)
	w.WriteHeader(http.StatusOK)
}

func CloseConnection(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(Response{
			Error: "Only POST method is allowed",
		})
		return
	}

	var req struct {
		ID string `json:"id"`
		IP string `json:"ip"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(Response{
			Error: "Invalid request body: " + err.Error(),
		})
		return
	}

	if req.ID == "" || req.IP == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(Response{
			Error: "ID and IP are required",
		})
		return
	}

	// Close the websocket connection
	server.CloseConnection(req.ID, req.IP)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(Response{
		Message: "Connection closed",
	})
}

func CloseServer(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(Response{
			Error: "Only POST method is allowed",
		})
		return
	}

	var req IPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(Response{
			Error: "Invalid request body: " + err.Error(),
		})
		return
	}

	if req.IP == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(Response{
			Error: "IP is required",
		})
		return
	}

	if err := server.CloseServer(req.IP); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(Response{
			Error: "Failed to close server: " + err.Error(),
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(Response{
		Message: "Server successfully closed",
	})
}
