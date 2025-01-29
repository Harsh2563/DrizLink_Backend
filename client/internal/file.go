package connection

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
)

func HandleSendFile(conn net.Conn, recipientId, filePath string) {
	file, err := os.Open(filePath)
	if err != nil {
		fmt.Println("error in open file", err)
		return
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		fmt.Println("error in stat file", err)
		return
	}

	fileSize := fileInfo.Size()
	fileName := fileInfo.Name()

	fmt.Printf("Sending file '%s' to user %s...\n", fileName, recipientId)
	// Send file request with file size
	_, err = conn.Write([]byte(fmt.Sprintf("/FILE_REQUEST %s %s %d\n", recipientId, fileName, fileSize)))
	if err != nil {
		fmt.Println("Error sending file request:", err)
		return
	}

	// Stream file data in chunks using io.CopyN
	n, err := io.CopyN(conn, file, fileSize)
	if err != nil {
		fmt.Println("error in write fileData", err)
		return
	}
	if n != fileSize {
		fmt.Println("error in copy file", err)
		return
	}
	fmt.Printf("File '%s' sent successfully with file size %d!\n", fileName, fileSize)
}

func HandleFileTransfer(conn net.Conn, recipientId, fileName string, fileSize int64, storeFilePath string) {
	fmt.Printf("Received file transfer response for: %s (Size: %d bytes)\n", storeFilePath, fileSize)
	filePath := storeFilePath + string(os.PathSeparator) + fileName
	file, err := os.Create(filePath)
	if err != nil {
		fmt.Println("error in create file", err)
		return
	}
	defer file.Close()

	_, err = io.CopyN(file, conn, fileSize)
	if err != nil {
		fmt.Println("error in write fileData", err)
		return
	}
	fmt.Printf("Received file transfer response for: %s (Size: %d bytes)\n", fileName, fileSize)
	fmt.Println("Starting file download...")
	fmt.Printf("File %s successfully received and saved to %s\n", fileName, storeFilePath)
}

func HandleDownloadRequest(conn net.Conn, recipientId, filePath string) {
	_, err := conn.Write([]byte(fmt.Sprintf("/DOWNLOAD_REQUEST %s %s\n", recipientId, filePath)))
	if err != nil {
		fmt.Println("Error sending file request:", err)
		return
	}
	fmt.Println("File download request sent successfully")
}

func HandleDownloadResponse(conn net.Conn, userId, filePath string) {
	cleanPath := filepath.Clean(strings.TrimSpace(filePath))
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		fmt.Printf("Error resolving absolute path: %v\n", err)
		return
	}

	fileInfo, err := os.Stat(absPath)
	if err != nil {
		fmt.Println("error in stat file", err)
		return
	}
	if !fileInfo.IsDir() {
		HandleSendFile(conn, userId, absPath)
	} else {
		HandleSendFolder(conn, userId, absPath)
	}
}
