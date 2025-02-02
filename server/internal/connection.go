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
	"time"

	"github.com/gorilla/websocket"
)

func Stop(server *interfaces.Server) error {
	server.Mutex.Lock()
	defer server.Mutex.Unlock()

	// Check if server is running
	if !server.Running {
		return fmt.Errorf("server is not running")
	}

	// Set server as not running first to prevent new connections
	server.Running = false

	// Close all existing connections
	for _, user := range server.Connections {
		if user.Conn != nil {
			go BroadcastMessage(interfaces.MessageData{
				Id:        helper.GenerateUserId(),
				Content:   "Server is not active",
				Sender:    "System",
				Timestamp: time.Now().Format(time.RFC3339),
			}, server,
				&interfaces.User{
					UserId:        user.UserId,
					IpAddress:     user.IpAddress,
					Username:      user.Username,
					StoreFilePath: user.StoreFilePath,
				})
			user.Conn.Close()
		}
	}

	// Close the HTTP server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.HttpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("server shutdown error: %v", err)
	}

	// Clean up server state
	server.Connections = make(map[string]*interfaces.User)
	server.IpAddresses = make(map[string]*interfaces.User)
	server.Address = ""
	server.Mux = nil
	server.HttpServer = nil

	return nil
}

func Start(server *interfaces.Server) error {
	server.Mutex.Lock()
	defer server.Mutex.Unlock()

	// Check if server is already running
	if server.Running {
		return fmt.Errorf("server is already running")
	}

	// Create a new ServeMux if none exists
	if server.Mux == nil {
		server.Mux = http.NewServeMux()
	}

	// Register the WebSocket route
	server.Mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := server.Upgrader.Upgrade(w, r, nil)
		if err != nil {
			fmt.Println("WebSocket upgrade failed:", err)
			return
		}
		defer conn.Close()
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
		if err := http.ListenAndServe(server.Address, server.Mux); err != nil {
			fmt.Println("WebSocket server failed to start:", err)
		}
	}()

	// Validate that the server is running
	time.Sleep(100 * time.Millisecond) // Give the server a moment to start
	_, err := net.Dial("tcp", server.Address)
	if err != nil {
		return fmt.Errorf("server failed to start: %v", err)
	}

	server.Running = true
	return nil
}

func StopUser(id string, server *interfaces.Server) error {
	server.Mutex.Lock()
	defer server.Mutex.Unlock()

	if !server.Running {
		return fmt.Errorf("server is not running")
	}

	user, ok := server.Connections[id]

	if !ok {
		return fmt.Errorf("user not found")
	}

	if user.Conn != nil {
		go BroadcastMessage(interfaces.MessageData{
			Id:        helper.GenerateUserId(),
			Content:   "User " + user.Username + " has left the chat",
			Sender:    "System",
			Timestamp: time.Now().Format(time.RFC3339),
		}, server,
			&interfaces.User{
				UserId:        user.UserId,
				IpAddress:     user.IpAddress,
				Username:      user.Username,
				StoreFilePath: user.StoreFilePath,
			})
		user.Conn.Close()
		delete(server.Connections, id)
		// delete(server.IpAddresses, user.IpAddress)
	}

	return nil
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
		return fmt.Errorf("server is not active")
	}
	server.Mutex.Unlock()

	ipAddr := conn.RemoteAddr().String()
	ip := strings.Split(ipAddr, ":")[0]
	fmt.Println("New connection from", ip)
	// if existingUser := server.IpAddresses[ip]; existingUser != nil {
	// 	fmt.Println("Connection already exists for IP:", ip)
	// 	// Send reconnection signal with existing user data
	// 	// reconnectMsg := fmt.Sprintf("/RECONNECT %s %s", existingUser.Username, existingUser.StoreFilePath)
	// 	// _, err := conn.Write([]byte(reconnectMsg))
	// 	// if err != nil {
	// 	// 	fmt.Println("Error sending reconnect signal:", err)
	// 	// 	return
	// 	// }

	// 	// Update connection and online status
	// 	server.Mutex.Lock()
	// 	existingUser.Conn = conn
	// 	existingUser.IsOnline = true
	// 	server.Mutex.Unlock()

	// 	// Encrypt and broadcast welcome back message
	// 	// welcomeMsg := fmt.Sprintf("User %s has rejoined the chat", existingUser.Username)
	// 	// BroadcastMessage(welcomeMsg, server)

	// 	// // Start handling messages for the reconnected user
	// 	// handleUserMessages(conn, existingUser, server)
	// 	return nil
	// }

	// Set read deadline for initial handshake
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	// Read username
	_, usernameBytes, err := conn.ReadMessage()
	if err != nil {
		fmt.Println("Failed to read username:", err)
		conn.Close()
		return fmt.Errorf("failed to read username: %v", err)
	}

	var userData interfaces.UserData
	if err := json.Unmarshal(usernameBytes, &userData); err != nil {
		fmt.Println("Invalid username format:", err)
		conn.Close()
		return fmt.Errorf("invalid username format: %v", err)
	}

	// Reset read deadline
	conn.SetReadDeadline(time.Time{})

	username := userData.Username
	storeFilePath := userData.FolderPath

	userId := helper.GenerateUserId()

	user := &interfaces.User{
		UserId:        userId,
		Username:      username,
		StoreFilePath: storeFilePath,
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
		Content:   fmt.Sprintf("User %s has joined the chat", username),
		Sender:    "System",
		Timestamp: time.Now().Format(time.RFC3339),
	}

	go BroadcastMessage(welcomeMsg, server, user)

	fmt.Printf("New user connected: %s (ID: %s)\n", username, userId)

	// Start handling messages for the new user
	handleUserMessages(conn, user, server)
	return nil
}

func handleUserMessages(conn *websocket.Conn, user *interfaces.User, server *interfaces.Server) error {
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			// Handle disconnect
			offlineMsg := interfaces.MessageData{
				Id:        helper.GenerateUserId(),
				Content:   fmt.Sprintf("User %s has disconnected", user.Username),
				Sender:    "System",
				Timestamp: time.Now().Format(time.RFC3339),
			}
			go BroadcastMessage(offlineMsg, server, user)
			return fmt.Errorf("user disconnected: %v", err)
		}

		var incomingMsg interfaces.MessageData
		if err := json.Unmarshal(message, &incomingMsg); err != nil {
			// Handle invalid message format
			errorMsg := interfaces.MessageData{
				Id:        helper.GenerateUserId(),
				Content:   "Invalid message format",
				Sender:    "System",
				Timestamp: time.Now().Format(time.RFC3339),
			}
			go BroadcastMessage(errorMsg, server, user)
			continue
		}

		// Validate required fields
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

		// Add server-side metadata
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

func BroadcastMessage(msgData interfaces.MessageData, server *interfaces.Server, sender *interfaces.User) {
	server.Mutex.Lock()
	defer server.Mutex.Unlock()

	jsonData, err := json.Marshal(msgData)
	if err != nil {
		fmt.Printf("Error marshalling message: %v\n", err)
		return
	}

	for userId, recipient := range server.Connections {
		// Skip if recipient is the sender or not online
		if recipient == sender || !recipient.IsOnline {
			continue
		}

		// Skip if connection is nil
		if recipient.Conn == nil {
			recipient.IsOnline = false
			delete(server.Connections, userId)
			continue
		}

		// Try to write the message
		err := recipient.Conn.WriteMessage(websocket.TextMessage, jsonData)
		if err != nil {
			if strings.Contains(err.Error(), "use of closed network connection") ||
				strings.Contains(err.Error(), "broken pipe") ||
				strings.Contains(err.Error(), "connection reset by peer") {
				// Connection is closed, update user status
				recipient.IsOnline = false
				delete(server.Connections, userId)
				if recipient.IpAddress != "" {
					delete(server.IpAddresses, recipient.IpAddress)
				}

				// Log the disconnection
				fmt.Printf("User %s disconnected, removing from active connections\n", recipient.Username)
			} else {
				// Log other types of errors
				fmt.Printf("Error broadcasting to %s: %v\n", recipient.Username, err)
			}
		}
	}
}

func StartHeartBeat(interval time.Duration, server *interfaces.Server) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			server.Mutex.Lock()
			for _, user := range server.Connections {
				if user.IsOnline {
					// Send proper ping message
					pingMsg := interfaces.MessageData{
						Id:        helper.GenerateUserId(),
						Content:   "ping",
						Sender:    "System",
						Timestamp: time.Now().Format(time.RFC3339),
					}
					jsonData, _ := json.Marshal(pingMsg)

					err := user.Conn.WriteControl(websocket.PingMessage, jsonData, time.Now().Add(time.Second))
					if err != nil {
						user.IsOnline = false
						offlineMsg := interfaces.MessageData{
							Id:        helper.GenerateUserId(),
							Content:   fmt.Sprintf("User %s is now offline", user.Username),
							Sender:    "System",
							Timestamp: time.Now().Format(time.RFC3339),
						}
						BroadcastMessage(offlineMsg, server, user)
					}
				}
			}
			server.Mutex.Unlock()
		}
	}()
}
