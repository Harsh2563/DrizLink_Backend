package helper

import (
	"archive/zip"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

func GenerateUserId() string {
	return strconv.Itoa(rand.Intn(10000000))
}

// Helper function to check if route is registered
func IsRouteRegistered(mux *http.ServeMux, path string) bool {
	if mux == nil {
		return false
	}

	req, _ := http.NewRequest("GET", path, nil)
	_, pattern := mux.Handler(req)
	return pattern == path
}

// CreateZipFromFolder creates a zip archive from a folder
func CreateZipFromFolder(folderPath string, zipPath string) error {
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return fmt.Errorf("failed to create zip file: %v", err)
	}
	defer zipFile.Close()

	archive := zip.NewWriter(zipFile)
	defer archive.Close()

	return filepath.Walk(folderPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Get the relative path for the zip
		relPath, err := filepath.Rel(folderPath, path)
		if err != nil {
			return err
		}

		// Skip if it's the root folder
		if relPath == "." {
			return nil
		}

		// Create zip header
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}

		// Set relative path as name
		header.Name = relPath

		// Set compression
		header.Method = zip.Deflate

		writer, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}

		// If it's a directory, just return
		if info.IsDir() {
			return nil
		}

		// Copy file contents
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(writer, file)
		return err
	})
}

// ExtractZip extracts a zip archive to the specified destination
func ExtractZip(zipPath string, destPath string) error {
	archive, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("failed to open zip file: %v", err)
	}
	defer archive.Close()

	for _, file := range archive.File {
		filePath := filepath.Join(destPath, file.Name)

		if file.FileInfo().IsDir() {
			os.MkdirAll(filePath, os.ModePerm)
			continue
		}

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
			return err
		}

		dstFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		if err != nil {
			return err
		}

		srcFile, err := file.Open()
		if err != nil {
			dstFile.Close()
			return err
		}

		_, err = io.Copy(dstFile, srcFile)
		srcFile.Close()
		dstFile.Close()

		if err != nil {
			return err
		}
	}

	return nil
}

// GetFolderSize returns the total size of a folder in bytes
func GetFolderSize(folderPath string) (int64, error) {
	var size int64
	err := filepath.Walk(folderPath, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}
