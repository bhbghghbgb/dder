package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"

	"resty.dev/v3"
)

// GameId represents the Game ID
type GameId struct {
	ID      string `json:"id"`  // Game ID
	GameBiz Biz    `json:"biz"` // Game business (uses custom Biz type), In original C# code, this is GameBiz, which is complex. For now, treat as a generic object.

}

// Biz is a custom struct to represent a string field
type Biz struct {
	Value string
}

// UnmarshalJSON implements the json.Unmarshaler interface for Biz
func (b *Biz) UnmarshalJSON(data []byte) error {
	// Decode the JSON string into the struct's Value field
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	b.Value = value
	return nil
}

// MarshalJSON implements the json.Marshaler interface (optional, for encoding back to JSON)
func (b Biz) MarshalJSON() ([]byte, error) {
	return json.Marshal(b.Value)
}

// GamePackage represents the game installation package information
type GamePackage struct {
	GameId      GameId             `json:"game"`         // Game ID
	Main        GamePackageVersion `json:"main"`         // Current version
	PreDownload GamePackageVersion `json:"pre_download"` // Pre-download version
}

// GamePackageVersion represents the installation package version
type GamePackageVersion struct {
	Major   *GamePackageResource  `json:"major"`   // Complete installation package, empty when no pre-download
	Patches []GamePackageResource `json:"patches"` // Incremental update packages
}

// GamePackageResource represents the installation package resources
type GamePackageResource struct {
	Version       string            `json:"version"`      // Version of the complete installation package or incremental update
	GamePackages  []GamePackageFile `json:"game_pkgs"`    // Main game files
	AudioPackages []GamePackageFile `json:"audio_pkgs"`   // Localization audio resources
	ResListUrl    string            `json:"res_list_url"` // Resource list URL (could be an empty string)
}

// GamePackageFile represents the game installation package file
type GamePackageFile struct {
	Language         *string `json:"language"`                 // Language for local audio resources (nullable)
	URL              string  `json:"url"`                      // Download URL
	MD5              string  `json:"md5"`                      // MD5 checksum (always lowercase)
	Size             int64   `json:"size,string"`              // Compressed package size (can be a string)
	DecompressedSize int64   `json:"decompressed_size,string"` // Decompressed size (can be a string)
}

// APIResponse represents the structure of the outer API response
type APIResponse struct {
	RetCode int     `json:"retcode"` // Return code
	Message string  `json:"message"` // Message
	Data    APIData `json:"data"`    // Data
}

type APIData struct {
	GamePackages []GamePackage `json:"game_packages"` // Array of GamePackage
}

func testGameAPICall() {
	// Initialize Resty client
	client := resty.New()

	// API URL
	url_b64 := "aHR0cHM6Ly9zZy1oeXAtYXBpLmhveW92ZXJzZS5jb20vaHlwL2h5cC1jb25uZWN0L2FwaS9nZXRHYW1lUGFja2FnZXM/bGF1bmNoZXJfaWQ9VllUcFhsYldvOA==" // don't github search me
	url, err := base64.StdEncoding.DecodeString(url_b64)

	if err != nil {
		log.Fatal("Can't decode api url:", err)
	}

	// Make the API call and automatically deserialize the result into APIResponse struct
	var apiResponse APIResponse
	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetResult(&apiResponse). // Deserializes the JSON response into the struct
		Get(string(url))

	if err != nil {
		log.Fatalf("Error while making API request: %v", err)
	}

	if resp.IsError() {
		log.Fatalf("API request failed with status code: %d, body: %s", resp.StatusCode(), resp.String())
		return
	}

	// Print response data
	fmt.Printf("Retcode: %d\n", apiResponse.RetCode)
	fmt.Printf("Message: %s\n", apiResponse.Message)
	fmt.Println("Game Packages:")
	for _, pkg := range apiResponse.Data.GamePackages {
		fmt.Printf("  Game ID: %+v\n", pkg.GameId)
		fmt.Printf("  Main Version: %+v\n", pkg.Main)
		fmt.Printf("  Pre-download Version: %+v\n", pkg.PreDownload)
		fmt.Println("  ---")
	}
}
