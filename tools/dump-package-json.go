package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/alexflint/go-arg"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/zeebo/xxh3"
)

// FileInfo holds metadata about a file.
// Hashes are stored as byte slices internally for memory efficiency.
// When writing to output, we convert them to hex strings.
// json tags determine the output field names when the struct is encoded to JSON.
// struct field names follow Go naming conventions (CamelCase)
// but are mapped to the requested JSON field names using `json:"..."` tags.
type FileInfo struct {
	FilePath  string // Relative path of the file from the input directory
	MD5       []byte // MD5 hash as a byte slice
	Xxh64Hash []byte // XXH64 hash as a byte slice
	Size      int64  // Size of the file in bytes
}

// FileInfoOutput is a struct specifically for the JSON output format.
// It contains the same information as FileInfo, but the hash values are stored as strings
// to be directly included in the JSON output.
type FileInfoOutput struct {
	FilePath  string `json:"remoteName"` // Path of the file, relative to the input directory
	MD5       string `json:"md5"`        // MD5 hash of the file as a hexadecimal string
	Xxh64Hash string `json:"hash"`       // XXH64 hash of the file as a hexadecimal string
	Size      int64  `json:"fileSize"`   // Size of the file in bytes
}

// Args is a struct that defines the command-line arguments.
// CLI flags: Define command-line arguments that the program can accept.
type Args struct {
	InputDir   string `arg:"positional,required" help:"Input directory to scan"`
	OutputFile string `arg:"-o,--output" default:"package.jsonl" help:"Output file (default: package.jsonl)"`
	Workers    int    `arg:"-w,--workers" default:"2" help:"Number of worker goroutines"` // Define the number of worker goroutines to run concurrently.
}

func main() {
	// Parse command-line arguments using the go-arg library.
	// This anonymous function is immediately invoked to parse the arguments and return the Args struct.
	args := func() Args {
		var args Args
		arg.MustParse(&args) // Populate the 'args' struct with values from command-line arguments.
		return args          // Return the populated 'args' struct.
	}()
	// Ensure that the input directory path uses forward slashes consistently,
	// regardless of the operating system's native path separator.
	args.InputDir = filepath.ToSlash(args.InputDir)
	// Ensure that the output file path also uses forward slashes consistently.
	args.OutputFile = filepath.ToSlash(args.OutputFile)

	// Check if the required input directory flag was provided.
	if args.InputDir == "" {
		log.Panic().Msg("Input directory is required") // If no input directory is given, log a fatal error and exit.
	}

	// Zerolog setup: Configure the logging library to output to the console.
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})

	// Channels for pipeline: Create channels to pass data between goroutines.
	paths := make(chan string, 10_000) // Buffered channel to send file paths from the walker to the workers.
	results := make(chan FileInfo, 8)  // Buffered channel to send processed file information from workers to the writer.

	// Start file walker: Launch a goroutine to traverse the input directory and send file paths to the 'paths' channel.
	go func() {
		defer close(paths)               // Ensure the 'paths' channel is closed when the file walker finishes. This signals to workers that no more paths will be sent.
		fileWalker(args.InputDir, paths) // Call the fileWalker function with the input directory and the paths channel.
	}()

	// Start workers: Launch a pool of worker goroutines to process the file paths received from the 'paths' channel.
	var workWg sync.WaitGroup // WaitGroup to wait for all worker goroutines to finish.
	workWg.Add(args.Workers)  // Add the number of workers to the WaitGroup counter.
	for range args.Workers {  // Iterate a fixed number of times (equal to args.Workers).
		go func() { // Launch an anonymous goroutine for each worker.
			defer workWg.Done()                       // Decrement the WaitGroup counter when the worker goroutine finishes.
			fileWorker(paths, args.InputDir, results) // Call the fileWorker function with the paths channel, input directory, and results channel.
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
		defer writeWg.Done()                   // Decrement the WaitGroup counter when the output writer goroutine finishes.
		outputWriter(args.OutputFile, results) // Call the outputWriter function with the output file path and the results channel.
	}()
	writeWg.Wait() // Wait for the output writer goroutine to finish writing all the results to the file.

	log.Info().Msg("Processing complete") // Log a message indicating that the entire process has finished.
}

// outputWriter creates the output file and launches the outputWorker goroutine.
func outputWriter(outputFile string, results <-chan FileInfo) {
	outFile, err := os.Create(outputFile) // Create (or truncate) the output file.
	if err != nil {
		log.Panic().Err(err).Msg("Failed to create output file") // If there's an error creating the file, log a fatal error and exit.
	}
	defer outFile.Close() // Ensure the output file is closed when this function returns.

	outputWorker(results, outFile) // Handle writing to the file.
}

// outputWorker reads FileInfo from the results channel, formats it as JSON, and writes it to the output file.
func outputWorker(results <-chan FileInfo, outFile *os.File) {
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

// fileWorker reads file paths from the paths channel, processes each file, and sends the FileInfo to the results channel.
func fileWorker(paths <-chan string, inputDir string, results chan<- FileInfo) {
	for path := range paths { // Continuously read file paths from the 'paths' channel until it's closed.
		info, err := processFile(inputDir, path) // Process the file to calculate hashes and size.
		if err != nil {
			log.Panic().Err(err).Str("file", path).Msg("Error processing file") // If there's an error processing the file, log a fatal error and exit.
			continue
		}
		results <- info // Send the processed FileInfo struct to the 'results' channel.
	}
}

// fileWalker recursively walks the input directory and sends the path of each file to the paths channel.
func fileWalker(inputDir string, paths chan<- string) {
	err := filepath.WalkDir(inputDir, func(path string, d os.DirEntry, err error) error { // WalkDir walks the file tree rooted at inputDir, calling the anonymous function for each file and directory.
		if err != nil {
			log.Panic().Err(err).Str("path", path).Msg("Error walking file") // If there's an error accessing a path, log a fatal error and return the error to stop walking.
			return nil
		}
		if !d.IsDir() { // Check if the current entry is a file (not a directory).
			paths <- path // Send the file path to the 'paths' channel for processing by workers.
			log.Debug().Str("file", path).Msg("Discovered")
		}
		return nil // Return nil to continue walking the directory tree.
	})
	if err != nil {
		log.Panic().Err(err).Msg("Failed to walk input directory") // If there's an error during the overall directory walk, log a fatal error and exit.
	}
}

// processFile reads the file and computes the MD5 and XXH64 hashes and the file size.
func processFile(baseDir string, path string) (FileInfo, error) {
	relPath, err := filepath.Rel(baseDir, path) // Get the relative path of the file with respect to the base directory.
	if err != nil {
		return FileInfo{}, err // If there's an error getting the relative path, return an empty FileInfo and the error.
	}
	// Convert to forward slashes for cross-platform consistency in the "remoteName" field.
	relPath = filepath.ToSlash(relPath)

	f, err := os.Open(path) // Open the file for reading.
	if err != nil {
		return FileInfo{}, err // If there's an error opening the file, return an empty FileInfo and the error.
	}
	defer f.Close() // Ensure the file is closed when this function returns.

	hMD5 := md5.New()    // Create a new MD5 hash object.
	hXXH64 := xxh3.New() // Create a new XXH3 hash object.

	// io.Copy reads from the file (f) and writes to both hash writers (hMD5 and hXXH64) simultaneously using MultiWriter.
	size, err := io.Copy(io.MultiWriter(hMD5, hXXH64), f)
	if err != nil {
		return FileInfo{}, err // If there's an error during the copying process, return an empty FileInfo and the error.
	}

	// Return a FileInfo struct containing the calculated metadata.
	log.Trace().Str("file", relPath).Msg("Processed")
	return FileInfo{
		FilePath:  relPath,         // The relative file path.
		MD5:       hMD5.Sum(nil),   // Calculate the final MD5 hash as a byte array.
		Xxh64Hash: hXXH64.Sum(nil), // Calculate the final XXH64 hash as a byte array.
		Size:      size,            // The size of the file in bytes.
	}, nil
}
