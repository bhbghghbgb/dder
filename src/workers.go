package main

import (
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// FileInfo struct to hold file properties
type FileInfo struct {
	Path string
	Size int64
	MD5  string
}

// FileInfoMap manages the concurrent map of file information
type FileInfoMap struct {
	Map map[string]FileInfo
}

// NewFileInfoMap creates a new FileInfoMap
func NewFileInfoMap() *FileInfoMap {
	return &FileInfoMap{Map: make(map[string]FileInfo)}
}

// DiscoverNewFile stores the initial file path
func (fim *FileInfoMap) DiscoverNewFile(path string) {
	if _, exists := fim.Map[path]; !exists {
		fim.Map[path] = FileInfo{Path: path}
	}
}

// UpdateSize updates the size of a file in the map
func (fim *FileInfoMap) UpdateSize(path string, size int64) {
	if value, exists := fim.Map[path]; exists {
		value.Size = size
	}
}

// UpdateMD5 updates the MD5 hash of a file in the map
func (fim *FileInfoMap) UpdateMD5(path string, md5Hash string) {
	if value, exists := fim.Map[path]; exists {
		value.MD5 = md5Hash
	}
}

// FileWorker processes files from the provided channel, calculates their size and MD5 hash,
// and updates the corresponding values using the provided callback functions.
//
// Args:
//
//	paths <-chan string:
//	  A channel that provides file paths to be processed.
//
//	updateSize func(string, int64):
//	  A callback function that is called with the file path and its size (in bytes).
//
//	updateMD5 func(string, string):
//	  A callback function that is called with the file path and its MD5 hash (as a hexadecimal string).
func FileWorker(paths <-chan string, updateSize func(string, int64), updateMD5 func(string, string)) {
	for path := range paths { // FileWorker consumes paths from here
		func() {
			file, err := os.Open(path)
			if err != nil {
				fmt.Println("Error opening file:", path, err)
				return
			}
			defer file.Close()

			fileStat, err := file.Stat()
			if err != nil {
				fmt.Println("Error getting file size:", path, err)
				return
			}
			size := fileStat.Size()
			updateSize(path, size)

			hash := md5.New()
			if _, err := io.Copy(hash, file); err != nil {
				fmt.Println("Error calculating MD5:", path, err)
				return
			}
			md5Hash := fmt.Sprintf("%x", hash.Sum(nil))
			updateMD5(path, md5Hash)
		}()
	}
}

// Launch the file walker goroutine
func FileWalker(root string, paths chan<- string, addFile func(string)) {
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Println("Error accessing path:", path, err)
			return nil // Continue walking despite the error
		}
		if !info.IsDir() {
			addFile(path) // Store initial info
			paths <- path // FileWalker sends paths to workers
		}
		return nil
	})
	if err != nil {
		fmt.Println("Error walking the directory:", err)
	}
}

func FileTest() {
	paths := make(chan string, 100) // Buffered channel
	numWorkers := 4                 // Number of workers
	var wg sync.WaitGroup

	// Log file discovery
	addFile := func(path string) {
		fmt.Println("Discovered file:", path)
	}

	// Log size updates
	updateSize := func(path string, size int64) {
		fmt.Printf("Updated size for %s: %d bytes\n", path, size)
	}

	// Log MD5 updates
	updateMD5 := func(path string, md5 string) {
		fmt.Printf("Updated MD5 for %s: %s\n", path, md5)
	}

	// Start FileWalker
	go func() {
		FileWalker(".", paths, addFile)
		close(paths) // Close the channel after FileWalker is done
	}()

	// Launch worker goroutines
	for range numWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			FileWorker(paths, updateSize, updateMD5)
		}()
	}
	wg.Wait() // Wait for all workers to finish
}
