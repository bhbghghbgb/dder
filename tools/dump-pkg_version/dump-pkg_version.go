package main

import (
	"crypto/md5"
	"io"
	"os"
	"path/filepath"

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
	Md5Hash   []byte // MD5 hash as a byte slice
	Xxh64Hash []byte // XXH64 hash as a byte slice
	Size      int64  // Size of the file in bytes
}

// FileInfoOutput is a struct specifically for the JSON output format.
// It contains the same information as FileInfo, but the hash values are stored as strings
// to be directly included in the JSON output.
type FileInfoOutput struct {
	FilePath  string `json:"remoteName"` // Path of the file, relative to the input directory
	Md5Hash   string `json:"md5"`        // MD5 hash of the file as a hexadecimal string
	Xxh64Hash string `json:"hash"`       // XXH64 hash of the file as a hexadecimal string
	Size      int64  `json:"fileSize"`   // Size of the file in bytes
}

// Args is the main struct that defines the top-level commands and global options.
type Args struct {
	Threads int        `arg:"-w,--workers" default:"2" help:"Number of worker goroutines for hashing"`
	Dump    *DumpCmd   `arg:"subcommand:dump"`
	Verify  *VerifyCmd `arg:"subcommand:verify"`
	Mirror  *MirrorCmd `arg:"subcommand:mirror"`
}

// DumpCmd defines the arguments for the "dump" subcommand.
type DumpCmd struct {
	InputDir   string `arg:"positional,required" help:"Input directory to scan"`
	OutputFile string `arg:"-o,--output" default:"package.jsonl" help:"Output file (default: package.jsonl)"`
}

// VerifyCmd defines the arguments for the "verify" subcommand.
type VerifyCmd struct {
	InputDir            string   `arg:"positional,required" help:"Input directory to scan"`
	PkgFiles            []string `arg:"-f,--pkg-file" help:"List of additional package files to use"`
	CheckInputDirForPkg bool     `arg:"-c,--check-input" help:"Look for pkg files in input directory"`
}

// MirrorCmd defines the arguments for the "mirror" subcommand.
type MirrorCmd struct {
	OutputDir string   `arg:"positional,required" help:"Output directory to create files to"`
	PkgFiles  []string `arg:"-f,--pkg-file" help:"List of additional package files to use"`
}

func main() {
	// Parse command-line arguments using the go-arg library.
	// This anonymous function is immediately invoked to parse the arguments and return the Args struct.
	args := func() Args {
		var args Args
		arg.MustParse(&args) // Populate the 'args' struct with values from command-line arguments.
		return args          // Return the populated 'args' struct.
	}()

	// Zerolog setup: Configure the logging library to output to the console.
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})

	switch {
	case args.Dump != nil:
		subcommandDump(&args, args.Dump)
	case args.Verify != nil:
		subcommandVerify(&args, args.Verify)
	case args.Mirror != nil:
		subcommandMirror(&args, args.Mirror)
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

// processFileReader computes the MD5 and XXH64 hashes and size from any io.Reader.
func processFileReader(reader io.Reader) (md5Hash []byte, xxh64Hash []byte, size int64, err error) {
	hMD5 := md5.New()    // Create a new MD5 hash object.
	hXXH64 := xxh3.New() // Create a new XXH3 hash object.

	// Use io.MultiWriter to write data simultaneously to both hash objects while reading.
	size, err = io.Copy(io.MultiWriter(hMD5, hXXH64), reader)
	if err != nil {
		return nil, nil, 0, err // If there's an error, return empty values and the error.
	}

	return hMD5.Sum(nil), hXXH64.Sum(nil), size, nil
}

// processFile reads the file and computes the MD5 and XXH64 hashes and file size.
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

	// Compute hashes and size using processFileReader.
	md5Hash, xxh64Hash, size, err := processFileReader(f)
	if err != nil {
		return FileInfo{}, err // If there's an error during processing, return an empty FileInfo and the error.
	}

	// Return a FileInfo struct containing the calculated metadata.
	log.Trace().Str("file", relPath).Msg("Done compare")
	return FileInfo{
		FilePath:  relPath,
		Md5Hash:   md5Hash,   // MD5 hash as a byte array.
		Xxh64Hash: xxh64Hash, // XXH64 hash as a byte array.
		Size:      size,      // File size in bytes.
	}, nil
}
