package main

import (
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"resty.dev/v3"
)

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})

	log.Info().Msg("Hello, World!")

	// testAPICall()
	// testGameAPICall()
	FileTest()
}

type Post struct {
	UserID int    `json:"userId"`
	ID     int    `json:"id"`
	Title  string `json:"title"`
	Body   string `json:"body"`
}

func testAPICall() {
	client := resty.New()
	defer client.Close()

	var posts []Post
	_, err := client.R().
		SetResult(&posts).
		Get("https://jsonplaceholder.typicode.com/posts")

	if err != nil {
		log.Err(err).Msg("Cannot reach Internet jsonplaceholder")
		return
	}

	log.Info().Msgf("Parsed Response: %+v\n", posts)
}
