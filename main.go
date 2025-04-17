package main

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"time"
)

// getNewsSummary executes the shell pipeline to fetch and summarize news.
func getNewsSummary() (string, error) {
	// Warning: This command relies on external tools (curl, strip-tags, ttok, llm) being installed.
	// It can also be slow and potentially expensive to run frequently.
	cmdStr := "curl -s https://www.ft.com/ | strip-tags .n-layout | ttok -t 4000 | llm -m 4o --system 'Create a concise summary that highlights the main points and crucial details of the provided news text. Eliminate unnecessary language and focus on the most important information use Headings followed by a short paragraph of concise text.'"
	cmd := exec.Command("bash", "-c", cmdStr)

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// Combine stderr with the error message for better debugging
		return "", fmt.Errorf("command execution failed: %w\nStderr: %s", err, stderr.String())
	}
	return out.String(), nil
}

// timeHandler writes an HTML page with the current time, news summary, and a meta refresh tag.
func timeHandler(w http.ResponseWriter, r *http.Request) {
	currentTime := time.Now().Format(time.RFC1123)

	// Get the news summary
	summary, err := getNewsSummary()
	summaryOutput := ""
	if err != nil {
		log.Printf("Error getting news summary: %v", err)
		// Display the error on the page for debugging
		summaryOutput = fmt.Sprintf("Error fetching summary:\n%v", err)
	} else {
		summaryOutput = summary
	}

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
    <hr>
    <h2>FT News Summary:</h2>
    <pre>%s</pre>
</body>
</html>
`, currentTime, summaryOutput)
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
