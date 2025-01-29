package main

import (
	"drizlink/server/interfaces"
	connection "drizlink/server/internal"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

func main() {
	//Intialize websocket upgrader
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
	server := interfaces.Server{
		Address:     ":8080",
		Connections: make(map[string]*interfaces.User),
		IpAddresses: make(map[string]*interfaces.User),
		Messages:    make(chan interfaces.Message),
		Upgrader:    upgrader,
	}

	go connection.StartHeartBeat(30*time.Second, &server)
	connection.Start(&server)
}
