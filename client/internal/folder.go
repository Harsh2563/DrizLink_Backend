package connection

import (
	"drizlink/helper"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
)

func HandleSendFolder(conn net.Conn, recipientId, folderPath string) {
	//Create a temperoroy zip file
	tempZipPath := folderPath + ".zip"
	err := helper.CreateZipFromFolder(folderPath, tempZipPath)
	if err != nil {
		fmt.Printf("Error creating zip file: ", err)
	}
	defer os.Remove(tempZipPath) //clean up temporary zip file

	//open zip file
	zipFile, err := os.Open(tempZipPath)
	if err != nil {
		fmt.Printf("Error opening temp zip file: ", err)
		return
	}

	defer zipFile.Close()

	//Get zip file info
	zipInfo, err := zipFile.Stat()
	if err != nil {
		fmt.Printf("Error getting zip file info: ", err)
		return
	}

	zipSize := zipInfo.Size()
	folderName := filepath.Base(folderPath)

	fmt.Printf("Sending folder '%s' to user %s...\n", folderName, recipientId)
	// Send folder request with zip size
	_, err = conn.Write([]byte(fmt.Sprintf("/FOLDER_REQUEST %s %s %d\n", recipientId, folderName, zipSize)))
	if err != nil {
		fmt.Printf("Error sending folder request: %v\n", err)
		return
	}

	//stream zip file data
	n, err := io.CopyN(conn, zipFile, zipSize)
	if err != nil {
		fmt.Printf("Error sending folder data: %v\n", err)
		return
	}
	if n != zipSize {
		fmt.Printf("Error: sent %d bytes, expected %d bytes\n", n, zipSize)
		return
	}
	fmt.Printf("Folder '%s' sent successfully!\n", folderName)
}

func HandleFolderTransfer(conn net.Conn, recipientId, folderName string, folderSize int64, storeFilePath string) {
	fmt.Printf("Receiving folder: %s (Size: %d bytes)\n", folderName, folderSize)

	// Create temporary zip file to store received data
	tempZipPath := filepath.Join(storeFilePath, folderName+".zip")
	zipFile, err := os.Create(tempZipPath)
	if err != nil {
		fmt.Printf("Error creating temporary zip file: %v\n", err)
		return
	}

	// Receive the zip file data
	n, err := io.CopyN(zipFile, conn, folderSize)
	if err != nil {
		zipFile.Close()
		os.Remove(tempZipPath)
		fmt.Printf("Error receiving folder data: %v\n", err)
		return
	}
	zipFile.Close()

	if n != folderSize {
		os.Remove(tempZipPath)
		fmt.Printf("Error: received %d bytes, expected %d bytes\n", n, folderSize)
		return
	}

	//Extract the zip file
	destPath := filepath.Join(storeFilePath, folderName)
	err = helper.ExtractZip(tempZipPath, destPath)
	if err != nil {
		os.Remove(tempZipPath)
		fmt.Printf("Error extracting folder: %v\n", err)
		return
	}

	// Clean up the temporary zip file
	os.Remove(tempZipPath)
	fmt.Printf("Folder '%s' received and extracted successfully!\n", folderName)
}

func HandleLookupRequest(conn net.Conn, userId string) {
	_, err := conn.Write([]byte(fmt.Sprintf("/LOOK %s\n", userId)))
	if err != nil {
		fmt.Printf("Error sending look request: %v\n", err)
		return
	}
}

func HandleLookupResponse(conn net.Conn, storeFilePath string, userId string) {
	// Clean and normalize the path
	cleanPath := filepath.Clean(strings.TrimSpace(storeFilePath))
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		fmt.Printf("Error resolving absolute path: %v\n", err)
		return
	}

	// Verify directory exists and is accessible
	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("Store directory does not exist: %s\n", absPath)
		} else {
			fmt.Printf("Error accessing directory: %v\n", err)
		}
		return
	}

	if !info.IsDir() {
		fmt.Printf("Path is not a directory: %s\n", absPath)
		return
	}

	var folders []string
	var files []string

	err = filepath.Walk(absPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Printf("Error accessing path %s: %v\n", path, err)
			return nil
		}
		if path == absPath {
			return nil
		}

		// Get clean relative path
		absolutePath := filepath.ToSlash(path)

		if info.IsDir() {
			folders = append(folders, fmt.Sprintf("[FOLDER] %s (Size: %d bytes)", absolutePath, info.Size()))
		} else {
			files = append(files, fmt.Sprintf("[FILE] %s (Size: %d bytes)", absolutePath, info.Size()))
		}
		return nil
	})

	if err != nil {
		fmt.Printf("Error walking directory: %v\n", err)
		return
	}

	var allEntries []string
	if len(folders) > 0 {
		allEntries = append(allEntries, "=== FOLDERS ===")
		allEntries = append(allEntries, folders...)
	}
	if len(files) > 0 {
		if len(allEntries) > 0 {
			allEntries = append(allEntries, "") // Add spacing between folders and files
		}
		allEntries = append(allEntries, "=== FILES ===")
		allEntries = append(allEntries, files...)
	}

	if len(allEntries) == 0 {
		allEntries = append(allEntries, "Directory is empty")
	}

	response := fmt.Sprintf("LOOK_RESPONSE %s %s\n", userId, strings.Join(allEntries, "\n"))
	_, err = conn.Write([]byte(response))
	if err != nil {
		fmt.Printf("Error sending lookup response: %v\n", err)
	}

	for _, entry := range allEntries {
		fmt.Println(entry)
	}
}
