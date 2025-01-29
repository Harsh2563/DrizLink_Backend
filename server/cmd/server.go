package main

import (
	"drizlink/server/interfaces"
	connection "drizlink/server/internal"
	"time"
)

func main() {
	server := interfaces.Server{
		Address:     "localhost:8080",
		Connections: make(map[string]*interfaces.User),
		IpAddresses: make(map[string]*interfaces.User),
		Messages:    make(chan interfaces.Message),
	}

	go connection.StartHeartBeat(100*time.Second, &server)
	connection.Start(&server)
}
