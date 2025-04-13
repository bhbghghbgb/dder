package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/rs/zerolog/log"
)

func subcommandDump(args *Args, dumpCmd *DumpCmd) {
	// Create local copies of args and dumpCmd to avoid unintended modifications.
	_args := *args
	_dumpCmd := *dumpCmd

	// Ensure that the input directory path uses forward slashes consistently,
	// regardless of the operating system's native path separator.
	_dumpCmd.InputDir = filepath.ToSlash(_dumpCmd.InputDir)
	// Ensure that the output file path also uses forward slashes consistently.
	_dumpCmd.OutputFile = filepath.ToSlash(_dumpCmd.OutputFile)

	// Check if the required input directory flag was provided.
	if _dumpCmd.InputDir == "" {
		log.Panic().Msg("Input directory is required") // If no input directory is given, log a fatal error and exit.
	}

	// Channels for pipeline: Create channels to pass data between goroutines.
	paths := make(chan string, 10_000) // Buffered channel to send file paths from the walker to the workers.
	results := make(chan FileInfo, 8)  // Buffered channel to send processed file information from workers to the writer.

	// Start file walker: Launch a goroutine to traverse the input directory and send file paths to the 'paths' channel.
	go func() {
		defer close(paths)                   // Ensure the 'paths' channel is closed when the file walker finishes. This signals to workers that no more paths will be sent.
		fileWalker(_dumpCmd.InputDir, paths) // Call the fileWalker function with the input directory and the paths channel.
	}()

	// Start workers: Launch a pool of worker goroutines to process the file paths received from the 'paths' channel.
	var workWg sync.WaitGroup // WaitGroup to wait for all worker goroutines to finish.
	workWg.Add(_args.Threads) // Add the number of workers to the WaitGroup counter.
	for range _args.Threads { // Iterate a fixed number of times (equal to _args.HashWorkers).
		go func() { // Launch an anonymous goroutine for each worker.
			defer workWg.Done()                           // Decrement the WaitGroup counter when the worker goroutine finishes.
			fileWorker(paths, _dumpCmd.InputDir, results) // Call the fileWorker function with the paths channel, input directory, and results channel.
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
		defer writeWg.Done()                       // Decrement the WaitGroup counter when the output writer goroutine finishes.
		pkgOutWriter(_dumpCmd.OutputFile, results) // Call the outputWriter function with the output file path and the results channel.
	}()
	writeWg.Wait() // Wait for the output writer goroutine to finish writing all the results to the file.
}

// pkgOutWriter creates the output file and launches the pkgOutWorker goroutine.
func pkgOutWriter(outputFile string, results <-chan FileInfo) {
	outFile, err := os.Create(outputFile) // Create (or truncate) the output file.
	if err != nil {
		log.Panic().Err(err).Msg("Failed to create output file") // If there's an error creating the file, log a fatal error and exit.
	}
	defer outFile.Close() // Ensure the output file is closed when this function returns.

	pkgOutWorker(results, outFile) // Handle writing to the file.
}

// pkgOutWorker reads FileInfo from the results channel, formats it as JSON, and writes it to the output file.
func pkgOutWorker(results <-chan FileInfo, outFile *os.File) {
	for result := range results { // Continuously read FileInfo structs from the 'results' channel until it's closed.
		// Convert hash bytes to hex strings for JSON output.
		out := FileInfoOutput{
			FilePath:  filepath.ToSlash(result.FilePath),    // Assign the file path. Convert to forward slashes for cross-platform consistency.
			MD5:       hex.EncodeToString(result.MD5),       // Convert the MD5 hash (byte array) to a hexadecimal string.
			Xxh64Hash: hex.EncodeToString(result.Xxh64Hash), // Convert the XXH64 hash (byte array) to a hexadecimal string.
			Size:      result.Size,                          // Assign the file size.
		}
		jsonBytes, err := json.Marshal(out) // Convert the FileInfoOutput struct to a JSON byte array.
		if err != nil {
			log.Panic().Err(err).Str("file", out.FilePath).Msg("Failed to marshal JSON") // If there's an error marshaling to JSON, log a fatal error and exit.
			continue
		}
		fmt.Fprintln(outFile, string(jsonBytes)) // Write the JSON string to the output file, adding a newline character.
		log.Info().
			Str("file", out.FilePath).
			Str("md5", out.MD5).
			Str("xxh64", out.Xxh64Hash).
			Int64("size", out.Size).
			Msg("Written")
	}
}
