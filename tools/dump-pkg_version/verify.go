package main

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
)

type CompareResult int

const (
	CR_Same CompareResult = iota
	CR_SizeDif
	CR_Md5Dif
	CR_Xxh64Dif
	CR_NotExist
	CR_IsDir
	CR_Error
)

type FileCompareResult struct {
	FilePath string // Relative path of the file from the input directory
	Result   CompareResult
}

func subcommandVerify(args *Args, verifyCmd *VerifyCmd) {
	// Create local copies of args and verifyCmd to avoid unintended modifications.
	_args := *args
	_verifyCmd := *verifyCmd

	// Ensure that the input directory path uses forward slashes consistently,
	// regardless of the operating system's native path separator.
	_verifyCmd.InputDir = filepath.ToSlash(_verifyCmd.InputDir)
	// Ensure that the output file path also uses forward slashes consistently.
	_verifyCmd.PkgFiles = lo.Map(_verifyCmd.PkgFiles, func(path string, _ int) string {
		return filepath.ToSlash(path)
	})

	// Check if the required input directory flag was provided.
	if _verifyCmd.InputDir == "" {
		log.Panic().Msg("Input directory is required") // If no input directory is given, log a fatal error and exit.
	}

	// it is pretty fast to read already, doesn't need multi thread as map will require locking anyway.
	pkgMap, err := readPkgFiles(_verifyCmd.InputDir, _verifyCmd.PkgFiles, _verifyCmd.CheckInputDirForPkg)
	if err != nil {
		log.Panic().Err(err).Msg("Error reading some pkg files")
	}

	var results []FileCompareResult
	var resultsMutex sync.Mutex
	workQueue := make(chan FileInfo, len(pkgMap)) // Work queue

	var workWg sync.WaitGroup
	// Start a fixed number of worker goroutines
	workWg.Add(_args.Threads)
	for range _args.Threads {
		go func() {
			defer workWg.Done()
			for file := range workQueue { // Workers pick tasks from the queue
				result, _ := compareFile(file)
				if result != CR_Same {
					resultsMutex.Lock()
					results = append(results, FileCompareResult{FilePath: file.FilePath, Result: result})
					resultsMutex.Unlock()
				}
			}
		}()
	}

	// Send work to the queue (no goroutine per file)
	for _, file := range pkgMap {
		workQueue <- file
	}
	pkgMap = nil // don't need the map anymore
	close(workQueue)
	workWg.Wait() // Wait for the comparator goroutines to finish writing all the results.

	for _, res := range results {
		baseLog := log.With().
			Str("file", res.FilePath).
			Logger()

		switch res.Result {
		case CR_Same:
			baseLog.Info().Msg("File is unchanged")
		case CR_SizeDif:
			baseLog.Info().Msg("File size differs")
		case CR_Md5Dif:
			baseLog.Info().Msg("MD5 hash differs")
		case CR_Xxh64Dif:
			baseLog.Info().Msg("XXH64 hash differs")
		case CR_NotExist:
			baseLog.Info().Msg("File does not exist")
		case CR_IsDir:
			baseLog.Warn().Msg("Path is a directory")
		case CR_Error:
			baseLog.Error().Msg("An error occurred while processing the file")
		default:
			baseLog.Error().Msg("Unknown result type")
		}
	}
}

func readPkgFile(baseDir string, pkgFilePath string, outMap map[string]FileInfo) error {
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
			FilePath:  filepath.Join(baseDir, fileInfoOutput.FilePath),
			Md5Hash:   decodeHex(fileInfoOutput.Md5Hash),
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
			err := readPkgFile(inputDir, fullPath, outMap)
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
		err := readPkgFile(inputDir, pkgFile, pkgMap)
		if err != nil {
			return nil, fmt.Errorf("error processing pkg file %s: %w", pkgFile, err)
		}
	}

	return pkgMap, nil
}

// compareFile reads the file and computes the MD5 and XXH64 hashes and file size.
func compareFile(file FileInfo) (CompareResult, error) {
	baseLog := log.With().Str("file", file.FilePath).Logger()
	baseLog.Trace().Msg("Start compare")
	f, err := os.Open(file.FilePath) // Open the file for reading.
	if err != nil {                  // If there's an error opening the file
		errno := err.(*os.PathError).Err.(syscall.Errno)
		switch errno {
		case syscall.ERROR_FILE_NOT_FOUND:
			baseLog.Info().Msg("File does not exist")
			return CR_NotExist, nil
		case syscall.EISDIR:
			baseLog.Warn().Msg("Path is a directory")
			return CR_IsDir, nil
		default:
			baseLog.Warn().Err(err).Msg("Unknown error")
			return CR_Error, err
		}
	}
	defer f.Close() // Ensure the file is closed when this function returns.

	stat, err := f.Stat()
	if err != nil {
		baseLog.Warn().Err(err).Msg("Failed to retrieve file metadata")
		return CR_Error, err
	}
	if stat.IsDir() {
		baseLog.Warn().Err(err).Msg("Path is a directory")
		return CR_IsDir, err
	}
	actualSize := stat.Size()
	if actualSize != file.Size {
		baseLog.Info().
			Int64("expected_size", file.Size).
			Int64("actual_size", actualSize).
			Msg("File size mismatch")
		return CR_SizeDif, nil
	}

	// Compute hashes and size using processFileReader.
	md5Hash, xxh64Hash, _, err := processFileReader(f)
	if err != nil {
		baseLog.Warn().Err(err).Msg("Error processing file hashes")
		return CR_Error, err // If there's an error during processing
	}
	if !bytes.Equal(md5Hash, file.Md5Hash) {
		baseLog.Info().
			Str("expected_md5", hex.EncodeToString(file.Md5Hash)).
			Str("actual_md5", hex.EncodeToString(md5Hash)).
			Msg("MD5 hash mismatch")
		return CR_Md5Dif, nil
	}
	if !bytes.Equal(xxh64Hash, file.Xxh64Hash) {
		baseLog.Info().
			Str("expected_xxh64", hex.EncodeToString(file.Xxh64Hash)).
			Str("actual_xxh64", hex.EncodeToString(xxh64Hash)).
			Msg("XXH64 hash mismatch")
		return CR_Xxh64Dif, nil
	}

	baseLog.Trace().Msg("File is unchanged")
	return CR_Same, nil
}
