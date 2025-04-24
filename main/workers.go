package main

import (
	"crypto/md5"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/rs/zerolog/log"
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
				log.Warn().Str("path", path).Err(err).Msg("Error opening file")
				return
			}
			defer file.Close()

			fileStat, err := file.Stat()
			if err != nil {
				log.Warn().Str("path", path).Err(err).Msg("Error getting file size")
				return
			}
			size := fileStat.Size()
			updateSize(path, size)

			hash := md5.New()
			if _, err := io.Copy(hash, file); err != nil {
				log.Warn().Str("path", path).Err(err).Msg("Error calculating MD5")
				return
			}
			md5Hash := hex.EncodeToString(hash.Sum(nil))
			updateMD5(path, md5Hash)
		}()
		// log.Info().Str("str", path).Msg("Consuming string")
	}
}

// Launch the file walker goroutine
func FileWalker(root string, paths chan<- string, addFile func(string)) {
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Panic().Str("path", path).Err(err).Msg("Error accessing path")
			return nil // Continue walking despite the error
		}
		if !info.IsDir() {
			addFile(path) // Store initial info
			paths <- path // FileWalker sends paths to workers
		}
		return nil
	})
	if err != nil {
		log.Panic().Err(err).Msg("Error walking the directory")
	}
}

func FileTest() {
	// Log file discovery
	addFile := func(path string) {
		log.Info().Str("file", path).Msg("Discovered file")
	}

	// Log size updates
	updateSize := func(path string, size int64) {
		log.Info().Str("file", path).Int64("size", size).Msg("Updated file size")
	}

	// Log MD5 updates
	updateMD5 := func(path string, md5 string) {
		log.Info().Str("file", path).Str("md5", md5).Msg("Updated file MD5")
	}

	// Using ChannelizedPriorityQueue as a middleman between two channels here to illustrate
	// how ChannelizedPriorityQueue will be sent items and and receive items from

	pathsFromProducer := make(chan string)
	// Start FileWalker
	go func() {
		FileWalker(".", pathsFromProducer, addFile)
		close(pathsFromProducer) // Close the channel after FileWalker is done
		log.Debug().Msg("pathsFromProducer closed")
	}()

	// Create a ChannelizedPriorityQueue to act as a middleman
	cpq := NewChannelizedPriorityQueue[string]()

	// Goroutine to transfer paths from the FileWalker to the ChannelizedPriorityQueue
	go func() {
		for path := range pathsFromProducer {
			item := &Item[string]{Value: path, Priority: len(path)} // Demo priority
			cpq.In() <- item
		}
		cpq.Close() // when pathsFromProducer is closed, close ChannelizedPriorityQueue also
		log.Debug().Msg("ChannelizedPriorityQueue closed")
	}()

	pathsToWorker := make(chan string)
	// Goroutine to transfer paths from the ChannelizedPriorityQueue to the workers
	go func() {
		for item := range cpq.Out() {
			pathsToWorker <- item.Value // Send the path to the workers
		}
		close(pathsToWorker) // Close it when ChannelizedPriorityQueue is closed
		log.Debug().Msg("pathsToWorker closed")

	}()

	numWorkers := 4 // Number of workers
	var wg sync.WaitGroup
	// Launch worker goroutines
	for range numWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			FileWorker(pathsToWorker, updateSize, updateMD5)
		}()
	}
	wg.Wait() // Wait for all workers to finish
}
