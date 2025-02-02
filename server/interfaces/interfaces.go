package interfaces

import (
	"net/http"
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
	HttpServer  *http.Server
	Mux         *http.ServeMux
	Running     bool
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

type UserData struct {
	Username   string
	FolderPath string
}

type MessageData struct {
	Id        string `json:"id"`
	Content   string `json:"content"`
	Sender    string `json:"sender"`
	Timestamp string `json:"timestamp"`
	File      *File  `json:"file,omitempty"`
}

type File struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	Type string `json:"type"`
}
