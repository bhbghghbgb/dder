package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
)

func subcommandMirror(args *Args, mirrorCmd *MirrorCmd) {
	// Create local copies of args and mirrorCmd to avoid unintended modifications.
	_args := *args
	_mirrorCmd := *mirrorCmd

	// Ensure that the input directory path uses forward slashes consistently,
	// regardless of the operating system's native path separator.
	_mirrorCmd.OutputDir = filepath.ToSlash(_mirrorCmd.OutputDir)
	// Ensure that the output file path also uses forward slashes consistently.
	_mirrorCmd.PkgFiles = lo.Map(_mirrorCmd.PkgFiles, func(path string, _ int) string {
		return filepath.ToSlash(path)
	})

	// Check if the required output directory flag was provided.
	if _mirrorCmd.OutputDir == "" {
		log.Panic().Msg("Output directory is required") // If no output directory is given, log a fatal error and exit.
	}

	// Initialize the map for storing FileInfoOutput objects
	pkgMap := make(map[string]FileInfoOutput)
	// it is pretty fast to read already, doesn't need multi thread as map will require locking anyway.
	// Process additional package files provided as flags
	for _, pkgFile := range _mirrorCmd.PkgFiles {
		err := readPkgFile(pkgFile, pkgMap)
		if err != nil {
			log.Panic().Err(err).Msg("Error reading some pkg files")
		}
	}

	workQueue := make(chan FileInfoOutput, len(pkgMap)) // Work queue

	var workWg sync.WaitGroup
	// Start a fixed number of worker goroutines
	workWg.Add(_args.Threads)
	for range _args.Threads {
		go func() {
			defer workWg.Done()
			for file := range workQueue { // Workers pick tasks from the queue
				mirrorFile(_mirrorCmd.OutputDir, file)
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
}

func mirrorFile(baseDir string, file FileInfoOutput) error {
	// Construct the output file path
	outputPath := filepath.Join(baseDir, file.FilePath+".json")

	// Create parent directories if they don't exist
	parentDir := filepath.Dir(outputPath)
	err := os.MkdirAll(parentDir, 0666)
	if err != nil {
		log.Warn().
			Err(err).
			Str("parentDir", parentDir).
			Msg("Failed to create directories")
		return err
	}

	// Marshal the FileInfo object to JSON
	data, err := json.Marshal(file)
	if err != nil {
		log.Panic().
			Err(err).
			Str("outputPath", outputPath).
			Str("filePath", file.FilePath).
			Msg("Failed to marshal FileInfo")
		return err
	}

	// Write the JSON data to the output file
	err = os.WriteFile(outputPath, data, 0666)
	if err != nil {
		log.Warn().
			Err(err).
			Str("outputPath", outputPath).
			Msg("Failed to write to file")
		return err
	}

	log.Debug().
		Str("file", outputPath).
		Msg("Wrote file")

	return nil
}
