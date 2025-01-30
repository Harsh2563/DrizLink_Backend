package server

import (
	"drizlink/server/interfaces"
	connection "drizlink/server/internal"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

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
	server := interfaces.Server{
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
