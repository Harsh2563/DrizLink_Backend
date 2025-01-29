package interfaces

import (
	"sync"

	"github.com/gorilla/websocket"
)

type Server struct {
	Address     string
	Connections map[string]*User
	IpAddresses map[string]*User
	Messages    chan Message
	Mutex       sync.Mutex
	Upgrader    websocket.Upgrader
}

type Message struct {
	SenderId       string
	SenderUsername string
	Content        string
	Timestamp      string
}

type User struct {
	UserId        string
	Username      string
	StoreFilePath string
	Conn          *websocket.Conn
	IsOnline      bool
	IpAddress     string
}
