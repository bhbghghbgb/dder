package main

import (
	"fmt"

	"resty.dev/v3"
)

func main() {
	fmt.Println("Hello, World!")

	testAPICall()
	testGameAPICall()
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
		fmt.Println("Error:", err)
		return
	}

	fmt.Printf("Parsed Response: %+v\n", posts)
}
