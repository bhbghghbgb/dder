package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
)

func subcommandCompare(args *Args, compareCmd *CompareCmd) {
	// Create local copies of args and compareCmd to avoid unintended modifications.
	_args := *args
	_compareCmd := *compareCmd

	// Ensure that the input directory path uses forward slashes consistently,
	// regardless of the operating system's native path separator.
	_compareCmd.InputDir = filepath.ToSlash(_compareCmd.InputDir)
	// Ensure that the output file path also uses forward slashes consistently.
	_compareCmd.PkgFiles = lo.Map(_compareCmd.PkgFiles, func(path string, _ int) string {
		return filepath.ToSlash(path)
	})

	// Check if the required input directory flag was provided.
	if _compareCmd.InputDir == "" {
		log.Panic().Msg("Input directory is required") // If no input directory is given, log a fatal error and exit.
	}

	// it is pretty fast to read already, doesn't need multi thread as map will require locking anyway.
	pkgMap, err := readPkgFiles(_compareCmd.InputDir, _compareCmd.PkgFiles, _compareCmd.Delete)
	if err != nil {
		log.Panic().Err(err).Msg("Error reading some pkg files")
	}

	// Channels for pipeline: Create channels to pass data between goroutines.
	paths := make(chan string, 10_000) // Buffered channel to send file paths from the walker to the workers.

	// Start file walker: Launch a goroutine to traverse the input directory and send file paths to the 'paths' channel.
	go func() {
		defer close(paths)                      // Ensure the 'paths' channel is closed when the file walker finishes. This signals to workers that no more paths will be sent.
		fileWalker(_compareCmd.InputDir, paths) // Call the fileWalker function with the input directory and the paths channel.
	}()

	// Start workers: Launch a pool of worker goroutines to process the file paths received from the 'paths' channel.
	var workWg sync.WaitGroup // WaitGroup to wait for all worker goroutines to finish.
	workWg.Add(_args.Threads) // Add the number of workers to the WaitGroup counter.
	for range _args.Threads { // Iterate a fixed number of times (equal to _args.Threads).
		go func() { // Launch an anonymous goroutine for each worker.
			defer workWg.Done()                              // Decrement the WaitGroup counter when the worker goroutine finishes.
			fileWorker(paths, _compareCmd.InputDir, results) // Call the fileWorker function with the paths channel, input directory, and results channel.
		}()
	}
	// Goroutine to close the results channel after all workers are done.
	go func() {
		workWg.Wait()  // Wait for all worker goroutines to finish processing files.
		close(results) // Close the 'results' channel. This signals to the output writer that no more results will be sent.
	}()

	// Start output writer: Launch a goroutine to read processed file information from the 'results' channel and write it to the output file.
	var writeWg sync.WaitGroup // WaitGroup to wait for the output writer goroutine to finish.
	writeWg.Add(1)             // Add 1 to the WaitGroup counter for the output writer goroutine.
	go func() {
		defer writeWg.Done()                          // Decrement the WaitGroup counter when the output writer goroutine finishes.
		pkgOutWriter(_compareCmd.OutputFile, results) // Call the outputWriter function with the output file path and the results channel.
	}()
	writeWg.Wait() // Wait for the output writer goroutine to finish writing all the results to the file.
}

func readPkgFile(pkgFilePath string, outMap map[string]FileInfo) error {
	file, err := os.Open(pkgFilePath)
	if err != nil {
		return fmt.Errorf("failed to open pkg file %s: %w", pkgFilePath, err)
	}
	defer file.Close()

	// Read and parse the file line by line
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		// Parse JSON into FileInfoOutput
		var fileInfoOutput FileInfoOutput
		err := json.Unmarshal([]byte(line), &fileInfoOutput)
		if err != nil {
			return fmt.Errorf("failed to unmarshal line in pkg file %s: %w", pkgFilePath, err)
		}

		// Convert FileInfoOutput to FileInfo
		fileInfo := FileInfo{
			FilePath:  fileInfoOutput.FilePath,
			MD5:       decodeHex(fileInfoOutput.MD5),
			Xxh64Hash: decodeHex(fileInfoOutput.Xxh64Hash),
			Size:      fileInfoOutput.Size,
		}

		// Store in map using remoteName as key
		outMap[fileInfoOutput.FilePath] = fileInfo
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading pkg file %s: %w", pkgFilePath, err)
	}

	return nil
}

// decodeHex converts a hex-encoded string into a byte slice
func decodeHex(hexStr string) []byte {
	decoded, err := hex.DecodeString(hexStr)
	if err != nil {
		log.Panic().Str("hex", hexStr).Msg("Failed to decode hex string")
	}
	return decoded
}

func scanInputDirForPkg(inputDir string, outMap map[string]FileInfo) error {
	entries, err := os.ReadDir(inputDir)
	if err != nil {
		return fmt.Errorf("failed to read input directory %s: %w", inputDir, err)
	}

	for _, entry := range entries {
		// Check for file names containing "pkg"
		if !entry.IsDir() && strings.Contains(entry.Name(), "pkg") {
			fullPath := filepath.Join(inputDir, entry.Name())

			// Read and process the pkg file
			err := readPkgFile(fullPath, outMap)
			if err != nil {
				return fmt.Errorf("error processing pkg file %s: %w", fullPath, err)
			}
		}
	}

	return nil
}

func readPkgFiles(inputDir string, pkgFiles []string, checkInputDirForPkg bool) (map[string]FileInfo, error) {
	// Initialize the map for storing FileInfo objects
	pkgMap := make(map[string]FileInfo)

	if checkInputDirForPkg {
		err := scanInputDirForPkg(inputDir, pkgMap)
		if err != nil {
			return nil, fmt.Errorf("failed to scan input directory for pkg files: %w", err)
		}
	}

	// Process additional package files provided as flags
	for _, pkgFile := range pkgFiles {
		err := readPkgFile(pkgFile, pkgMap)
		if err != nil {
			return nil, fmt.Errorf("error processing pkg file %s: %w", pkgFile, err)
		}
	}

	return pkgMap, nil
}
