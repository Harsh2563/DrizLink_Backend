package connection

import (
	"context"
	"drizlink/helper"
	"drizlink/server/interfaces"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	MaxConnections       = 100              // Maximum allowed connections
	HandshakeTimeout     = 10 * time.Second // Timeout for initial handshake
	BroadcastTimeout     = 5 * time.Second  // Timeout for broadcasting messages
	HeartbeatInterval    = 30 * time.Second // Interval for heartbeat checks
	ConnectionBufferSize = 1024 * 1024      // 1MB buffer size for connections
)

var (
	ErrServerNotRunning     = fmt.Errorf("server is not running")
	ErrServerAlreadyRunning = fmt.Errorf("server is already running")
	ErrUserNotFound         = fmt.Errorf("user not found")
	ErrMaxConnections       = fmt.Errorf("maximum connections reached")
)

// Stop shuts down the server and cleans up resources.
func Stop(server *interfaces.Server) error {
	server.Mutex.Lock()
	defer server.Mutex.Unlock()

	// Check if server is running
	if !server.Running {
		return ErrServerNotRunning
	}

	// Set server as not running first to prevent new connections
	server.Running = false

	// Close all existing connections
	var wg sync.WaitGroup
	for _, user := range server.Connections {
		if user.Conn != nil {
			wg.Add(1)
			go func(u *interfaces.User) {
				defer wg.Done()
				u.Conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseGoingAway, "Server shutting down"), time.Now().Add(BroadcastTimeout))
				u.Conn.Close()
			}(user)
		}
	}
	wg.Wait()

	// Shutdown the HTTP server
	ctx, cancel := context.WithTimeout(context.Background(), BroadcastTimeout)
	defer cancel()
	if err := server.HttpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("server shutdown error: %v", err)
	}

	// Clean up resources
	server.Connections = make(map[string]*interfaces.User)
	server.IpAddresses = make(map[string]*interfaces.User)
	server.Address = ""
	server.Mux = nil
	server.HttpServer = nil

	return nil
}

// Start starts the WebSocket server.
func Start(server *interfaces.Server) error {
	server.Mutex.Lock()
	defer server.Mutex.Unlock()

	// Check if server is already running
	if server.Running {
		return ErrServerAlreadyRunning
	}

	// Create a new ServeMux if none exists
	if server.Mux == nil {
		server.Mux = http.NewServeMux()
	}

	// Register the WebSocket route
	server.Mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		if len(server.Connections) >= MaxConnections {
			http.Error(w, "Too many connections", http.StatusServiceUnavailable)
			return
		}
		conn, err := server.Upgrader.Upgrade(w, r, nil)
		if err != nil {
			fmt.Println("WebSocket upgrade failed:", err)
			return
		}
		defer conn.Close()

		// Set connection buffer size
		conn.SetReadLimit(ConnectionBufferSize)

		// Start handling the connection
		if err := HandleConnection(conn, server); err != nil {
			fmt.Println("Error handling connection:", err)
		}
	})

	// Create HTTP server instance
	server.HttpServer = &http.Server{
		Addr:    server.Address,
		Handler: server.Mux,
	}

	// Start the server in a goroutine
	go func() {
		fmt.Println("WebSocket server starting on", server.Address)
		if err := server.HttpServer.ListenAndServe(); err != nil {
			fmt.Println("WebSocket server failed to start:", err)
		}
	}()

	// Validate that the server is running
	time.Sleep(100 * time.Millisecond) // Give the server a moment to start
	if _, err := net.Dial("tcp", server.Address); err != nil {
		return fmt.Errorf("server failed to start: %v", err)
	}

	server.Running = true
	return nil
}

// StopUser disconnects a specific user and cleans up their resources.
func StopUser(id string, server *interfaces.Server) (*interfaces.User, error) {
	server.Mutex.Lock()
	defer server.Mutex.Unlock()

	if !server.Running {
		return nil, ErrServerNotRunning
	}

	user, ok := server.Connections[id]
	if !ok {
		return nil, ErrUserNotFound
	}

	if user.Conn != nil {
		user.Conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseGoingAway, "User disconnected"), time.Now().Add(BroadcastTimeout))
		user.Conn.Close()
	}

	// Create a copy of the user before deletion
	userCopy := &interfaces.User{
		UserId:        user.UserId,
		IpAddress:     user.IpAddress,
		Username:      user.Username,
		StoreFilePath: user.StoreFilePath,
	}

	delete(server.Connections, id)
	delete(server.IpAddresses, user.IpAddress)

	return userCopy, nil
}

func HandleConnection(conn *websocket.Conn, server *interfaces.Server) error {
	server.Mutex.Lock()
	if !server.Running {
		server.Mutex.Unlock()
		message := interfaces.MessageData{
			Id:        helper.GenerateUserId(),
			Content:   "Server is not active",
			Sender:    "System",
			Timestamp: time.Now().Format(time.RFC3339),
		}
		go BroadcastMessage(message, server, nil)
		conn.Close()
		return ErrServerNotRunning
	}
	server.Mutex.Unlock()

	ipAddr := conn.RemoteAddr().String()
	ip := strings.Split(ipAddr, ":")[0]
	fmt.Println("New connection from", ip)

	// Check if user is already connected
	if existingUser := server.IpAddresses[ip]; existingUser != nil {
		server.Mutex.Lock()
		// Close old connection if it exists
		if existingUser.Conn != nil {
			existingUser.Conn.Close()
		}
		// Update connection and online status
		existingUser.Conn = conn
		existingUser.IsOnline = true
		server.Mutex.Unlock()

		// Broadcast reconnection message
		welcomeMsg := interfaces.MessageData{
			Id:        helper.GenerateUserId(),
			Content:   fmt.Sprintf("User %s has reconnected", existingUser.Username),
			Sender:    "System",
			Timestamp: time.Now().Format(time.RFC3339),
		}
		go BroadcastMessage(welcomeMsg, server, existingUser)

		handleUserMessages(conn, existingUser, server)
		return nil
	}

	// Set read deadline for initial handshake
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	// Read username
	_, userInfoBytes, err := conn.ReadMessage()
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to read username: %v", err)
	}

	var userData interfaces.UserData
	if err := json.Unmarshal(userInfoBytes, &userData); err != nil {
		conn.Close()
		return fmt.Errorf("invalid username format: %v", err)
	}

	// Reset read deadline
	conn.SetReadDeadline(time.Time{})

	// Create new user
	user := &interfaces.User{
		UserId:        helper.GenerateUserId(),
		Username:      userData.Username,
		StoreFilePath: userData.FolderPath,
		Conn:          conn,
		IsOnline:      true,
		IpAddress:     ip,
	}

	server.Mutex.Lock()
	server.Connections[user.UserId] = user
	server.IpAddresses[ip] = user
	server.Mutex.Unlock()

	welcomeMsg := interfaces.MessageData{
		Id:        helper.GenerateUserId(),
		Content:   fmt.Sprintf("User %s has joined the chat", user.Username),
		Sender:    "System",
		Timestamp: time.Now().Format(time.RFC3339),
	}

	go BroadcastMessage(welcomeMsg, server, user)

	fmt.Printf("New user connected: %s (ID: %s)\n", user.Username, user.UserId)

	// Start handling messages for the new user
	handleUserMessages(conn, user, server)
	return nil
}

func handleUserMessages(conn *websocket.Conn, user *interfaces.User, server *interfaces.Server) error {
	defer func() {
		server.Mutex.Lock()
		delete(server.Connections, user.UserId)
		delete(server.IpAddresses, user.IpAddress)
		server.Mutex.Unlock()

		offlineMsg := interfaces.MessageData{
			Id:        helper.GenerateUserId(),
			Content:   fmt.Sprintf("User %s has disconnected", user.Username),
			Sender:    "System",
			Timestamp: time.Now().Format(time.RFC3339),
		}
		go BroadcastMessage(offlineMsg, server, user)
	}()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("user disconnected: %v", err)
		}

		var incomingMsg interfaces.MessageData
		if err := json.Unmarshal(message, &incomingMsg); err != nil {
			errorMsg := interfaces.MessageData{
				Id:        helper.GenerateUserId(),
				Content:   "Invalid message format",
				Sender:    "System",
				Timestamp: time.Now().Format(time.RFC3339),
			}
			go BroadcastMessage(errorMsg, server, user)
			continue
		}

		if incomingMsg.Content == "" || incomingMsg.Sender == "" {
			errorMsg := interfaces.MessageData{
				Id:        helper.GenerateUserId(),
				Content:   "Message missing required fields",
				Sender:    "System",
				Timestamp: time.Now().Format(time.RFC3339),
			}
			go BroadcastMessage(errorMsg, server, user)
			continue
		}

		outgoingMsg := interfaces.MessageData{
			Id:        helper.GenerateUserId(),
			Content:   incomingMsg.Content,
			Sender:    user.Username,
			Timestamp: time.Now().Format(time.RFC3339),
			File:      incomingMsg.File,
		}
		go BroadcastMessage(outgoingMsg, server, user)
	}
}

// BroadcastMessage sends a message to all connected users.
func BroadcastMessage(msgData interfaces.MessageData, server *interfaces.Server, sender *interfaces.User) {
	server.Mutex.Lock()
	defer server.Mutex.Unlock()

	jsonData, err := json.Marshal(msgData)
	if err != nil {
		fmt.Printf("Error marshalling message: %v\n", err)
		return
	}

	for _, user := range server.Connections {
		if user == sender || !user.IsOnline || user.Conn == nil {
			continue
		}

		if err := user.Conn.WriteMessage(websocket.TextMessage, jsonData); err != nil {
			fmt.Printf("Error broadcasting to %s: %v\n", user.Username, err)
			user.IsOnline = false
			delete(server.Connections, user.UserId)
			delete(server.IpAddresses, user.IpAddress)
		}
	}
}

// StartHeartBeat periodically checks the health of connections.
func StartHeartBeat(interval time.Duration, server *interfaces.Server) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			server.Mutex.Lock()
			for _, user := range server.Connections {
				if user.IsOnline {
					if err := user.Conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(BroadcastTimeout)); err != nil {
						user.IsOnline = false
						offlineMsg := interfaces.MessageData{
							Id:        helper.GenerateUserId(),
							Content:   fmt.Sprintf("User %s is now offline", user.Username),
							Sender:    "System",
							Timestamp: time.Now().Format(time.RFC3339),
						}
						go BroadcastMessage(offlineMsg, server, user)
					}
				}
			}
			server.Mutex.Unlock()
		}
	}()
}
