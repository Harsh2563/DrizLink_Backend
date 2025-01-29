package connection

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

func Connect(address string) (net.Conn, error) {
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func Close(conn net.Conn) {
	conn.Close()
}

func UserInput(attribute string, conn net.Conn) error {
	// First check if we get a reconnection signal
	buffer := make([]byte, 1024)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := conn.Read(buffer)
	conn.SetReadDeadline(time.Time{}) // Reset read deadline

	if err == nil && n > 0 {
		message := string(buffer[:n])
		if strings.HasPrefix(message, "/RECONNECT") {
			parts := strings.SplitN(message, " ", 4)
			if len(parts) == 3 {
				fmt.Printf("Welcome back %s!\n", parts[1])
				return errors.New("reconnect")
			}
		}
	}

	// If no reconnection signal, proceed with normal user input
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("Enter your " + attribute + ": ")
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	_, err = conn.Write([]byte(input))
	if err != nil {
		fmt.Println("error in write " + attribute)
		panic(err)
	}

	return nil
}

func ReadLoop(conn net.Conn) {
	for {
		buffer := make([]byte, 1024)
		n, err := conn.Read(buffer)
		if err != nil {
			fmt.Println("error in read")
			return
		}
		message := string(buffer[:n])
		switch {
		case strings.HasPrefix(message, "/FILE_RESPONSE"):
			fmt.Println("File transfer response received")
			args := strings.SplitN(message, " ", 5)
			if len(args) != 5 {
				fmt.Println("Invalid arguments. Use: /FILE_RESPONSE <userId> <filename> <fileSize> <storeFilePath>")
				continue
			}
			recipientId := args[1]
			fileName := args[2]
			fileSizeStr := strings.TrimSpace(args[3])
			fileSize, err := strconv.ParseInt(fileSizeStr, 10, 64)
			storeFilePath := args[4]
			if err != nil {
				fmt.Println("Invalid fileSize. Use: /FILE_RESPONSE <userId> <filename> <fileSize> <storeFilePath>")
				continue
			}

			HandleFileTransfer(conn, recipientId, fileName, int64(fileSize), storeFilePath)
			continue
		case strings.HasPrefix(message, "/FOLDER_RESPONSE"):
			fmt.Println("Folder transfer response received")
			args := strings.SplitN(message, " ", 5)
			if len(args) != 5 {
				fmt.Println("Invalid arguments. Use: /FOLDER_RESPONSE <userId> <folderName> <folderSize> <storeFilePath>")
				continue
			}
			recipientId := args[1]
			folderName := args[2]
			folderSizeStr := strings.TrimSpace(args[3])
			folderSize, err := strconv.ParseInt(folderSizeStr, 10, 64)
			storeFilePath := args[4]
			if err != nil {
				fmt.Println("Invalid folderSize. Use: /FOLDER_RESPONSE <userId> <folderName> <folderSize> <storeFilePath>")
				continue
			}
			HandleFolderTransfer(conn, recipientId, folderName, folderSize, storeFilePath)
			continue
		case strings.HasPrefix(message, "PING"):
			_, err = conn.Write([]byte("PONG\n"))
			if err != nil {
				fmt.Println("Error responding to heartbeat: ", err)
				continue
			}
		case message == "USERS:":
			buffer := make([]byte, 1024)
			n, err := conn.Read(buffer)
			if err != nil {
				fmt.Println("error in read message", err)
				continue
			}
			userList := string(buffer[:n])
			fmt.Println(userList)
			continue
		case strings.HasPrefix(message, "/LOOK_REQUEST"):
			args := strings.SplitN(message, " ", 3)
			if len(args) != 3 {
				fmt.Println("Invalid arguments. Use: /LOOK_REQUEST <storageFilePath> <userId>")
				continue
			}
			storageFilePath := args[2]
			userId := args[1]
			HandleLookupResponse(conn, storageFilePath, userId)
			continue
		case strings.HasPrefix(message, "/LOOK_RESPONSE"):
			args := strings.SplitN(message, " ", 3)
			if len(args) != 3 {
				fmt.Println("Invalid arguments. Use: /LOOK_RESPONSE <userId> <files>")
				continue
			}
			userId := args[1]
			files := strings.Split(args[2], " ")

			fmt.Println("\n----------------------------")
			fmt.Printf(" Directory Listing for User: %s\n", userId)
			fmt.Println("----------------------------")

			for _, file := range files {
				if strings.HasSuffix(file, "/") {
					fmt.Printf("📁 %s\n", file) // Folder display with emoji
				} else {
					fmt.Printf("  📄 %s\n", file) // File display with emoji
				}
			}

			fmt.Println("----------------------------\n")
			continue
		case strings.HasPrefix(message, "/DOWNLOAD_REQUEST"):
			args := strings.SplitN(message, " ", 3)
			if len(args) != 3 {
				fmt.Println("Invalid arguments. Use: /DOWNLOAD_REQUEST <userId> <filename>")
				continue
			}
			userId := args[1]
			filePath := args[2]
			HandleDownloadResponse(conn, userId, filePath)
			continue
		default:
			fmt.Println(message)
		}
	}
}

func WriteLoop(conn net.Conn) {
	reader := bufio.NewReader(os.Stdin)
	for {
		message, _ := reader.ReadString('\n')
		message = strings.TrimSpace(message)
		switch {
		case message == "exit":
			fmt.Println("Goodbye!")
			conn.Close()
			return
		case strings.HasPrefix(message, "/sendfile"):
			args := strings.SplitN(message, " ", 3)
			if len(args) != 3 {
				fmt.Println("Invalid arguments. Use: /sendfile <userId> <filename>")
				continue
			}
			recipientId := args[1]
			filePath := args[2]
			HandleSendFile(conn, recipientId, filePath)
			continue
		case strings.HasPrefix(message, "/sendfolder"):
			args := strings.SplitN(message, " ", 3)
			if len(args) != 3 {
				fmt.Println("Invalid arguments. Use: /sendfolder <userId> <folderPath>")
				continue
			}
			recipientId := args[1]
			folderPath := args[2]
			HandleSendFolder(conn, recipientId, folderPath)
			continue
		case strings.HasPrefix(message, "/lookup"):
			args := strings.SplitN(message, " ", 2)
			if len(args) != 2 {
				fmt.Println("Invalid arguments. Use: /lookup <userId>")
				continue
			}
			recipientId := args[1]
			HandleLookupRequest(conn, recipientId)
			continue
		case strings.HasPrefix(message, "/status"):
			_, err := conn.Write([]byte(message))
			if err != nil {
				fmt.Println("error in write message", err)
				continue
			}
			continue
		case strings.HasPrefix(message, "/download"):
			args := strings.SplitN(message, " ", 3)
			if len(args) != 3 {
				fmt.Println("Invalid arguments. Use: /download <userId> <filename>")
				continue
			}
			recipientId := args[1]
			filePath := args[2]
			HandleDownloadRequest(conn, recipientId, filePath)
			continue
		default:
			_, err := conn.Write([]byte(message))
			if err != nil {
				fmt.Println("error in write message", err)
				return
			}
		}
	}
}
