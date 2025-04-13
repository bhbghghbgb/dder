package hyapi

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/samber/lo"
	"resty.dev/v3"
)

// DryRunTransport simulates HTTP responses by returning predefined responses.
type DryRunTransport struct {
	MockResponseFile string // Path to the mock response file
	StatusCode       int    // HTTP status code to return
}

// RoundTrip provides the fake HTTP response.
func (drt *DryRunTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	// Open the mock response file
	file, err := os.Open(drt.MockResponseFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open mock response file: %w", err)
	}
	defer file.Close()

	// Read the file contents into memory
	mockData, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read mock response file: %w", err)
	}

	// Create the fake HTTP response
	return &http.Response{
		StatusCode: drt.StatusCode,
		Body:       io.NopCloser(bytes.NewReader(mockData)),
		Header:     make(http.Header),
	}, nil
}

func TestCallAPI_GetGamePackages(t *testing.T) {
	// Create a Resty client and use the DryRunTransport
	client := resty.New()
	client.SetTransport(&DryRunTransport{
		MockResponseFile: "../../res/demoapi/getGamePackages.json", // The mock response file
		StatusCode:       http.StatusOK,                            // Simulate HTTP 200 OK
	})

	defer client.Close()

	// Get raw response
	gamePackages := callAPIWithClient[[]GamePackage](client, "sg-hyp-api.hoyoverse.com", "getGamePackages", "VYTpXlbWo8", "en-us", "game_packages")

	// Perform assertions
	if len(gamePackages) == 0 {
		t.Fatalf("Expected non-empty result, got empty")
	}

	// Locate the specific game package
	// Locate the specific game package using "lo" library
	targetPackage, ok := lo.Find(gamePackages, func(pkg GamePackage) bool {
		return pkg.GameId.ID == "gopR6Cufr3" && pkg.GameId.GameBiz == "hk4e_global"
	})
	if !ok {
		t.Fatalf("Target game package not found")
	}

	// Confirm size of audio_pkgs is always "4"
	if len(targetPackage.Main.Major.AudioPackages) != 4 {
		t.Errorf("Expected audio_pkgs size to be 4, but got %d", len(targetPackage.Main.Major.AudioPackages))
	}

	// Confirm that all game_pkgs.url under the major version contains the version string
	version := targetPackage.Main.Major.Version
	if !lo.EveryBy(targetPackage.Main.Major.GamePackages, func(gamePkg GamePackageFile) bool {
		return strings.Contains(gamePkg.URL, version)
	}) {
		t.Fatalf("Not all URLs contain version %s", version)
	}
}
