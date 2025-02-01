package server

import (
	"drizlink/server/interfaces"
	connection "drizlink/server/internal"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

var server interfaces.Server

func StartServer(ip string) error {
	//Intialize websocket upgrader
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}

	//Initialize server
	server = interfaces.Server{
		Address:     ip + ":8080",
		Connections: make(map[string]*interfaces.User),
		IpAddresses: make(map[string]*interfaces.User),
		Messages:    make(chan interfaces.Message),
		Upgrader:    upgrader,
	}

	//Start heartbeat
	go connection.StartHeartBeat(30*time.Second, &server)

	//Start server
	err := connection.Start(&server)
	if err != nil {
		return fmt.Errorf("failed to start server: %v", err)
	}

	return nil
}

func GetConnectedUsers() []interfaces.User {
	server.Mutex.Lock()
	defer server.Mutex.Unlock()
	users := make([]interfaces.User, 0, len(server.Connections))
	for _, user := range server.Connections {
		users = append(users, *user)
	}
	return users
}

func CloseConnection(ip string) {
	server.Mutex.Lock()
	defer server.Mutex.Unlock()
	if user, ok := server.Connections[ip]; ok {
		user.Conn.Close()
		delete(server.Connections, ip)
		delete(server.IpAddresses, ip)
	}
}
