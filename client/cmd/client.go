package main

import (
	connection "drizlink/client/internal"
	"fmt"
)

func main() {
	conn, err := connection.Connect("192.168.0.203:8080")
	if err != nil {
		if err.Error() == "reconnect" {
			goto startChat
		} else {
			panic(err)
		}
	}

	defer connection.Close(conn)

	err = connection.UserInput("Username", conn)
	if err != nil {
		if err.Error() == "reconnect" {
			goto startChat
		} else {
			panic(err)
		}
	}

	// Only ask for store file path for new connections
	err = connection.UserInput("Store File Path", conn)
	if err != nil {
		if err.Error() == "reconnect" {
			goto startChat
		} else {
			panic(err)
		}
	}

startChat:
	fmt.Println("\nWelcome to the P2P File Sharing App!")
	fmt.Println("-----------------------------------")
	fmt.Println("Type '/status' to see online users.")
	fmt.Println("Use '/sendfile <userId> <filename>' to send a file.")
	fmt.Println("Type 'exit' to quit.")

	go connection.ReadLoop(conn)
	connection.WriteLoop(conn)
}
