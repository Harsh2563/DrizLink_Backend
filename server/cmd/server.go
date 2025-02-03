package server

import (
	"drizlink/server/interfaces"
	connection "drizlink/server/internal"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var (
	servers      map[string]*interfaces.Server // Map to store server instances
	serversMutex sync.RWMutex                  // Mutex to protect concurrent access to the map
)

// Initialize the servers map when the package is loaded
func init() {
	servers = make(map[string]*interfaces.Server)
}

func StartServer(ip string) error {
	// Validate the IP address
	if ip == "" {
		return errors.New("IP address cannot be empty")
	}

	// Calculate full server address
	address := ip + ":8080"

	// Check if the server already exists
	serversMutex.Lock()
	defer serversMutex.Unlock()

	server, exists := servers[address]
	if exists {
		if server.Running {
			return errors.New("server is already running. Connect as a client to the server")
		}
		server.Running = true
		return nil
	}

	// Initialize WebSocket upgrader
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}

	// Initialize a new server instance
	newServer := &interfaces.Server{
		ServerId:    address,
		Address:     address,
		Connections: make(map[string]*interfaces.User),
		IpAddresses: make(map[string]*interfaces.User),
		Messages:    make(chan interfaces.Message),
		Upgrader:    upgrader,
		Mutex:       sync.Mutex{},
	}

	// Start the heartbeat mechanism
	go connection.StartHeartBeat(30*time.Second, newServer)

	// Start the server
	err := connection.Start(newServer)
	if err != nil {
		return fmt.Errorf("failed to start server: %v", err)
	}

	// Store the new server in the map
	servers[newServer.Address] = newServer
	return nil
}

func GetConnectedUsers(serverId string) ([]interfaces.User, error) {
	serversMutex.RLock()
	defer serversMutex.RUnlock()

	server, exists := servers[serverId]
	if !exists {
		return nil, errors.New("server not found")
	}

	if !server.Running {
		return nil, errors.New("server is not running")
	}

	server.Mutex.Lock()
	defer server.Mutex.Unlock()

	users := make([]interfaces.User, 0, len(server.Connections))
	for _, user := range server.Connections {
		users = append(users, *user)
	}
	return users, nil
}

func CloseConnection(userId string, serverId string) (*interfaces.User, error) {
	serversMutex.RLock()
	defer serversMutex.RUnlock()

	server, exists := servers[serverId]
	if !exists {
		return nil, errors.New("server not found")
	}
	if !server.Running {
		return nil, errors.New("server is not running")
	}
	return connection.StopUser(userId, server)
}

func CloseServer(serverId string) error {
	serversMutex.Lock()
	defer serversMutex.Unlock()

	server, exists := servers[serverId]
	if !exists {
		return errors.New("server not found")
	}
	if !server.Running {
		return errors.New("server is not running")
	}
	if err := connection.Stop(server); err != nil {
		return err
	}
	delete(servers, serverId)
	return nil
}
