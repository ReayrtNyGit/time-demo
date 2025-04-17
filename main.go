package main

import (
	"fmt"
	"log"
	"net/http"
	"time"
)

// timeHandler writes an HTML page with the current time and a meta refresh tag.
func timeHandler(w http.ResponseWriter, r *http.Request) {
	currentTime := time.Now().Format(time.RFC1123)
	// Set the content type to HTML
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Write the HTML response
	fmt.Fprintf(w, `
<!DOCTYPE html>
<html>
<head>
    <title>Current Time</title>
    <meta http-equiv="refresh" content="1">
</head>
<body>
    <h1>The current time is: %s</h1>
</body>
</html>
`, currentTime)
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
