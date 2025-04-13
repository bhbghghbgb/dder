package hyapi

type GamePackages struct {
	GamePackages []GamePackage `json:"game_packages"` // Array of GamePackage
}

// GameId represents the Game ID
type GameId struct {
	ID      string `json:"id"`  // Game ID
	GameBiz string `json:"biz"` // Game business.
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

func getGamePackages() []GamePackage {
	// Default values
	hostname := "sg-hyp-api.hoyoverse.com"
	api := "getGamePackages"
	launcherID := "VYTpXlbWo8"
	language := "en-us"
	nestedKey := "game_packages"

	// Fetch game packages from the API
	result := callAPI[[]GamePackage](hostname, api, launcherID, language, nestedKey)

	return result
}
