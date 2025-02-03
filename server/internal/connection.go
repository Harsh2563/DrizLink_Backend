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

// WebSocket Message Types
const (
	MsgTypeSystem    = "system"
	MsgTypeFileReq   = "file_request"
	MsgTypeFileRes   = "file_response"
	MsgTypeFolderReq = "folder_request"
	MsgTypeFolderRes = "folder_response"
	MsgTypeLookupReq = "lookup_request"
	MsgTypeLookupRes = "lookup_response"
	MsgTypeHeartbeat = "heartbeat"
	MsgTypeUserList  = "user_list"
	MsgTypeError     = "error"
)

type FileRequest struct {
	RecipientID string `json:"recipientId"`
	Filename    string `json:"filename"`
	Filesize    int64  `json:"filesize"`
	FilePath    string `json:"filePath"`
}

type FileResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type WSMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type LookupRequest struct {
	UserID string `json:"userId"`
}

type LookupResponse struct {
	UserID string      `json:"userId"`
	Files  []FileEntry `json:"files"`
}

type FileEntry struct {
	Type string `json:"type"`
	Path string `json:"path"`
	Size int64  `json:"size"`
	Name string `json:"name"`
}

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
				msg := WSMessage{
					Type:    MsgTypeSystem,
					Payload: json.RawMessage(`{"message": "Server shutting down"}`),
				}
				u.Conn.WriteJSON(msg)
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
		conn.Close()
		return ErrServerNotRunning
	}
	server.Mutex.Unlock()

	ipAddr := conn.RemoteAddr().String()
	ip := strings.Split(ipAddr, ":")[0]
	fmt.Println("New connection from", ip)

	// Check if user is already connected
	// if existingUser := server.IpAddresses[ip]; existingUser != nil {
	// 	server.Mutex.Lock()
	// 	// Close old connection if it exists
	// 	if existingUser.Conn != nil {
	// 		existingUser.Conn.Close()
	// 	}
	// 	// Update connection and online status
	// 	existingUser.Conn = conn
	// 	existingUser.IsOnline = true
	// 	server.Mutex.Unlock()

	// 	// Broadcast reconnection message
	// 	welcomeMsg := interfaces.MessageData{
	// 		Id:        helper.GenerateUserId(),
	// 		Content:   fmt.Sprintf("User %s has reconnected", existingUser.Username),
	// 		Sender:    "System",
	// 		Timestamp: time.Now().Format(time.RFC3339),
	// 	}
	// 	go BroadcastMessage(welcomeMsg, server, existingUser)

	// 	handleUserMessages(conn, existingUser, server)
	// 	return nil
	// }

	// Set read deadline for initial handshake
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	var msg WSMessage
	if err := conn.ReadJSON(&msg); err != nil {
		fmt.Println("Error reading initial message:", err)
		return err
	}
	// Validate message type
	if msg.Type != "connection-request" {
		sendError(conn, "invalid initial message type")
		conn.Close()
		return fmt.Errorf("invalid initial message type: %s", msg.Type)
	}

	var userData struct {
		ID         string `json:"id"`
		Username   string `json:"username"`
		FolderPath string `json:"folderPath"`
	}

	if err := json.Unmarshal(msg.Payload, &userData); err != nil {
		sendError(conn, "invalid user data format")
		return err
	}

	// Create user object
	user := &interfaces.User{
		UserId:        userData.ID,
		Username:      userData.Username,
		StoreFilePath: userData.FolderPath,
		Conn:          conn,
		IsOnline:      true,
		IpAddress:     strings.Split(conn.RemoteAddr().String(), ":")[0],
	}

	// Add to connections
	server.Mutex.Lock()
	server.Connections[user.UserId] = user
	server.IpAddresses[user.IpAddress] = user
	server.Mutex.Unlock()

	// Send confirmation

	sendSystemMessage(server, fmt.Sprintf("User %s connected", user.Username))
	conn.SetReadDeadline(time.Time{}) // Reset timeout

	// Start message handler
	go handleUserMessages(conn, user, server)

	fmt.Println(server.Connections)

	return nil
}

func handleUserMessages(conn *websocket.Conn, user *interfaces.User, server *interfaces.Server) error {
	defer func() {
		server.Mutex.Lock()
		delete(server.Connections, user.UserId)
		delete(server.IpAddresses, user.IpAddress)
		server.Mutex.Unlock()
		conn.Close()
		sendSystemMessage(server, fmt.Sprintf("User %s has left", user.Username))
	}()

	for {
		var msg WSMessage
		err := conn.ReadJSON(&msg)
		if err != nil {
			// Check if it's a normal closure error
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				fmt.Printf("Connection closed unexpectedly: %v\n", err)
			}
			return nil // Return normally instead of propagating the error
		}

		switch msg.Type {
		case MsgTypeFileReq:
			var req FileRequest
			if err := json.Unmarshal(msg.Payload, &req); err != nil {
				sendError(conn, "invalid file request format")
				continue
			}
			HandleFileRequest(server, user, req)

		case MsgTypeLookupReq:
			var req LookupRequest
			if err := json.Unmarshal(msg.Payload, &req); err != nil {
				sendError(conn, "invalid lookup request format")
				continue
			}
			HandleLookupRequest(server, user, req)

		case MsgTypeHeartbeat:
			conn.WriteJSON(WSMessage{Type: MsgTypeHeartbeat})

		default:
			sendError(conn, "unknown message type")
		}
	}
}

// HandleFileRequest handles file transfer requests
func HandleFileRequest(server *interfaces.Server, sender *interfaces.User, req FileRequest) {
	recipient, exists := server.Connections[req.RecipientID]
	if !exists {
		sendError(sender.Conn, "user not found")
		return
	}

	msg := WSMessage{
		Type: MsgTypeFileReq,
		Payload: json.RawMessage(fmt.Sprintf(`{
			"senderId": "%s",
			"filename": "%s",
			"filesize": %d,
			"filepath": "%s"
		}`, sender.UserId, req.Filename, req.Filesize, req.FilePath)),
	}

	if err := recipient.Conn.WriteJSON(msg); err != nil {
		sendError(sender.Conn, "failed to send file request")
	}
}

// HandleLookupRequest handles directory lookup requests
func HandleLookupRequest(server *interfaces.Server, sender *interfaces.User, req LookupRequest) {
	targetUser, exists := server.Connections[req.UserID]
	if !exists {
		sendError(sender.Conn, "user not found")
		return
	}

	msg := WSMessage{
		Type:    MsgTypeLookupReq,
		Payload: json.RawMessage(fmt.Sprintf(`{"requestorId": "%s"}`, sender.UserId)),
	}

	if err := targetUser.Conn.WriteJSON(msg); err != nil {
		sendError(sender.Conn, "failed to send lookup request")
	}
}

// sendError sends an error message through the WebSocket connection
func sendError(conn *websocket.Conn, message string) {
	msg := WSMessage{
		Type: MsgTypeError,
		Payload: json.RawMessage(fmt.Sprintf(`{
			"message": "%s",
			"timestamp": "%s"
		}`, message, time.Now().Format(time.RFC3339))),
	}
	conn.WriteJSON(msg)
}

func sendSystemMessage(server *interfaces.Server, content string) {
	msg := WSMessage{
		Type: MsgTypeSystem,
		Payload: json.RawMessage(fmt.Sprintf(`{
			"id": "%s",
			"content": "%s",
			"timestamp": "%s"
		}`, helper.GenerateUserId(), content, time.Now().Format(time.RFC3339))),
	}

	server.Mutex.Lock()
	defer server.Mutex.Unlock()

	for _, user := range server.Connections {
		if user.Conn != nil && user.IsOnline {
			user.Conn.WriteJSON(msg)
		}
	}
}

// BroadcastMessage sends a message to all connected users.
// func BroadcastMessage(msgData interfaces.MessageData, server *interfaces.Server, sender *interfaces.User) {
// 	server.Mutex.Lock()
// 	defer server.Mutex.Unlock()

// 	jsonData, err := json.Marshal(msgData)
// 	if err != nil {
// 		fmt.Printf("Error marshalling message: %v\n", err)
// 		return
// 	}

// 	for _, user := range server.Connections {
// 		if user == sender || !user.IsOnline || user.Conn == nil {
// 			continue
// 		}

// 		if err := user.Conn.WriteMessage(websocket.TextMessage, jsonData); err != nil {
// 			fmt.Printf("Error broadcasting to %s: %v\n", user.Username, err)
// 			user.IsOnline = false
// 			delete(server.Connections, user.UserId)
// 			delete(server.IpAddresses, user.IpAddress)
// 		}
// 	}
// }

// StartHeartBeat periodically checks the health of connections.
func StartHeartBeat(interval time.Duration, server *interfaces.Server) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			server.Mutex.Lock()
			for _, user := range server.Connections {
				if user.IsOnline {
					msg := WSMessage{Type: MsgTypeHeartbeat}
					if err := user.Conn.WriteJSON(msg); err != nil {
						user.IsOnline = false
						if websocket.IsUnexpectedCloseError(err) {
							fmt.Printf("Connection closed for user %s\n", user.Username)
						}
						sendSystemMessage(server, fmt.Sprintf("User %s disconnected", user.Username))
					}
				}
			}
			server.Mutex.Unlock()
		}
	}()
}
