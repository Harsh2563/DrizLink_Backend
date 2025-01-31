package connection

import (
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
		HandleConnection(conn, server)
	})

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

func HandleConnection(conn *websocket.Conn, server *interfaces.Server) {
	ipAddr := conn.RemoteAddr().String()
	ip := strings.Split(ipAddr, ":")[0]
	fmt.Println("New connection from", ip)
	// if existingUser := server.IpAddresses[ip]; existingUser != nil {
	// 	fmt.Println("Connection already exists for IP:", ip)
	// 	// Send reconnection signal with existing user data
	// 	reconnectMsg := fmt.Sprintf("/RECONNECT %s %s", existingUser.Username, existingUser.StoreFilePath)
	// 	_, err := conn.Write([]byte(reconnectMsg))
	// 	if err != nil {
	// 		fmt.Println("Error sending reconnect signal:", err)
	// 		return
	// 	}

	// 	// Update connection and online status
	// 	server.Mutex.Lock()
	// 	existingUser.Conn = conn
	// 	existingUser.IsOnline = true
	// 	server.Mutex.Unlock()

	// 	// Encrypt and broadcast welcome back message
	// 	welcomeMsg := fmt.Sprintf("User %s has rejoined the chat", existingUser.Username)
	// 	BroadcastMessage(welcomeMsg, server)

	// 	// Start handling messages for the reconnected user
	// 	handleUserMessages(conn, existingUser, server)
	// 	return
	// }

	// Set read deadline for initial handshake
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	// Read username
	_, usernameBytes, err := conn.ReadMessage()
	if err != nil {
		fmt.Println("Failed to read username:", err)
		conn.Close()
		return
	}

	var userData interfaces.UserData
	if err := json.Unmarshal(usernameBytes, &userData); err != nil {
		fmt.Println("Invalid username format:", err)
		conn.Close()
		return
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

	welcomeMsg := fmt.Sprintf("User %s has joined the chat", username)
	var msgData interfaces.MessageData
	msgData.Content = welcomeMsg
	msgData.Sender = username
	msgData.Timestamp = time.Now().Format(time.RFC3339)
	msgData.Id = user.UserId
	jsonData, err := json.Marshal(msgData)
	if err != nil {
		fmt.Println("Error marshalling message:", err)
		return
	}

	BroadcastMessage(string(jsonData), server, user)

	fmt.Printf("New user connected: %s (ID: %s)\n", username, userId)

	// Start handling messages for the new user
	handleUserMessages(conn, user, server)
}

func handleUserMessages(conn *websocket.Conn, user *interfaces.User, server *interfaces.Server) {
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			fmt.Printf("User disconnected: %s\n", user.Username)
			server.Mutex.Lock()
			user.IsOnline = false
			server.Mutex.Unlock()
			offlineMsg := fmt.Sprintf("User %s is now offline", user.Username)
			BroadcastMessage(offlineMsg, server, user)
			return
		}

		messageContent := string(message)

		switch {
		// case messageContent == "/exit":
		// 	server.Mutex.Lock()
		// 	user.IsOnline = false
		// 	server.Mutex.Unlock()
		// 	offlineMsg := fmt.Sprintf("User %s is now offline", user.Username)
		// 	BroadcastMessage(offlineMsg, server, user)
		// 	return
		// case strings.HasPrefix(messageContent, "/FILE_REQUEST"):
		// 	args := strings.SplitN(messageContent, " ", 4)
		// 	if len(args) != 4 {
		// 		fmt.Println("Invalid arguments. Use: /FILE_REQUEST <userId> <filename> <fileSize>")
		// 		continue
		// 	}
		// 	recipientId := args[1]
		// 	fileName := args[2]
		// 	fileSizeStr := strings.TrimSpace(args[3])
		// 	fileSize, err := strconv.ParseInt(fileSizeStr, 10, 64)
		// 	if err != nil {
		// 		fmt.Println("Invalid fileSize. Use: /FILE_REQUEST <userId> <filename> <fileSize>")
		// 		continue
		// 	}

		// 	HandleFileTransfer(server, conn, recipientId, fileName, fileSize)
		// 	continue
		// case strings.HasPrefix(messageContent, "/FOLDER_REQUEST"):
		// 	args := strings.SplitN(messageContent, " ", 4)
		// 	if len(args) != 4 {
		// 		fmt.Println("Invalid arguments. Use: /FOLDER_REQUEST <userId> <folderName> <folderSize>")
		// 		continue
		// 	}
		// 	recipientId := args[1]
		// 	folderName := args[2]
		// 	folderSizeStr := strings.TrimSpace(args[3])
		// 	folderSize, err := strconv.ParseInt(folderSizeStr, 10, 64)
		// 	if err != nil {
		// 		fmt.Println("Invalid folderSize. Use: /FOLDER_REQUEST <userId> <folderName> <folderSize>")
		// 		continue
		// 	}

		// 	// HandleFolderTransfer(server, conn, recipientId, folderName, folderSize)
		// 	continue
		// case messageContent == "PONG\n":
		// 	continue
		// case strings.HasPrefix(messageContent, "/status"):
		// 	_, err = conn.Write([]byte("USERS:"))
		// 	if err != nil {
		// 		fmt.Println("Error sending user list header:", err)
		// 		continue
		// 	}
		// 	for _, user := range server.Connections {
		// 		if user.IsOnline {
		// 			statusMsg := fmt.Sprintf("%s is online\n", user.Username)
		// 			_, err = conn.Write([]byte(statusMsg))
		// 			if err != nil {
		// 				fmt.Println("Error sending user list:", err)
		// 				continue
		// 			}
		// 		}
		// 	}
		// 	continue
		// case strings.HasPrefix(messageContent, "/LOOK"):
		// 	args := strings.SplitN(messageContent, " ", 2)
		// 	if len(args) != 2 {
		// 		fmt.Println("Invalid arguments. Use: /LOOK <userId>")
		// 		continue
		// 	}
		// 	recipientId := strings.TrimSpace(args[1])
		// 	HandleLookupRequest(server, conn, recipientId)
		// 	continue
		// case strings.HasPrefix(messageContent, "/DIR_LISTING"):
		// 	args := strings.SplitN(messageContent, " ", 3)
		// 	if len(args) != 3 {
		// 		fmt.Println("Invalid arguments. Use: /DIR_LISTING <userId> <files>")
		// 		continue
		// 	}
		// 	userId := strings.TrimSpace(args[1])
		// 	files := strings.TrimSpace(args[2])
		// 	HandleLookupResponse(server, conn, userId, strings.Split(files, " "))
		// 	continue
		// case strings.HasPrefix(messageContent, "/DOWNLOAD_REQUEST"):
		// 	args := strings.SplitN(messageContent, " ", 3)
		// 	if len(args) != 3 {
		// 		fmt.Println("Invalid arguments. Use: /DOWNLOAD_REQUEST <userId> <filename>")
		// 		continue
		// 	}
		// 	senderId := strings.TrimSpace(args[1])
		// 	recipientId := user.UserId
		// 	filePath := strings.TrimSpace(args[2])
		// 	HandleDownloadRequest(server, conn, senderId, recipientId, filePath)
		// 	continue
		default:
			BroadcastMessage(messageContent, server, user)
		}
	}
}

func BroadcastMessage(content string, server *interfaces.Server, sender *interfaces.User) {
	server.Mutex.Lock()
	defer server.Mutex.Unlock()
	for _, recipient := range server.Connections {
		if recipient.IsOnline && recipient != sender {
			err := recipient.Conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("%s: %s\n", sender.Username, content)))
			if err != nil {
				fmt.Printf("Error broadcasting message to %s: %v\n", recipient.Username, err)
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
					err := user.Conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(time.Second))
					if err != nil {
						fmt.Printf("User disconnected: %s\n", user.Username)
						user.IsOnline = false
						BroadcastMessage(fmt.Sprintf("User %s is now offline", user.Username), server, user)
					}
				}
			}
			server.Mutex.Unlock()
		}
	}()
}
