package main

import (
	"fmt"
	"log"
	"net/http"
	"time"
)

// timeHandler writes the current time to the HTTP response.
func timeHandler(w http.ResponseWriter, r *http.Request) {
	currentTime := time.Now().Format(time.RFC1123)
	fmt.Fprintf(w, "The current time is: %s", currentTime)
}

func main() {
	// Register the timeHandler function for the root path.
	http.HandleFunc("/", timeHandler)

	// Define the port the server will listen on.
	port := ":8080"
	log.Printf("Server starting on port %s\n", port)

	// Start the HTTP server.
	// http.ListenAndServe always returns a non-nil error.
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatalf("Could not start server: %s\n", err)
	}
}
