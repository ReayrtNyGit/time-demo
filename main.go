package main

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"sync"
	"time"
)

// Global variables for caching the news summary
var (
	cachedSummary        string
	lastSummaryUpdateTime time.Time
	summaryMutex         sync.RWMutex // Read-write mutex for safe concurrent access
	summaryFetchError    error        // Store potential error during fetch
)

// fetchAndSummarizeNews executes the shell pipeline to fetch and summarize news.
// This function actually performs the fetch operation.
func fetchAndSummarizeNews() (string, error) {
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

// getLatestNewsSummary returns the cached summary if it's recent,
// otherwise triggers a new fetch.
func getLatestNewsSummary() (string, error) {
	summaryMutex.RLock() // Acquire read lock to check time
	needsUpdate := time.Since(lastSummaryUpdateTime) > time.Hour || cachedSummary == ""
	summaryMutex.RUnlock() // Release read lock

	if needsUpdate {
		summaryMutex.Lock() // Acquire write lock for potential update
		// Double-check if another goroutine updated it while waiting for the lock
		if time.Since(lastSummaryUpdateTime) > time.Hour || cachedSummary == "" {
			log.Println("News summary cache expired or empty. Fetching new summary...")
			summary, err := fetchAndSummarizeNews()
			if err != nil {
				log.Printf("Error fetching news summary: %v", err)
				// Keep the stale cache but store the error
				summaryFetchError = err
				// Optionally, clear the cache on error: cachedSummary = ""
			} else {
				cachedSummary = summary
				summaryFetchError = nil // Clear previous error on success
			}
			lastSummaryUpdateTime = time.Now() // Update time even if fetch failed to prevent constant retries
		}
		summaryMutex.Unlock() // Release write lock
	}

	// Return the current cache content and any stored error
	summaryMutex.RLock()
	defer summaryMutex.RUnlock()
	// If there was an error during the last fetch attempt, report it along with potentially stale data
	if summaryFetchError != nil {
		errorMsg := fmt.Sprintf("Error during last summary fetch attempt: %v\n(Showing potentially stale data below)\n\n%s", summaryFetchError, cachedSummary)
		return errorMsg, summaryFetchError // Return error state
	}
	return cachedSummary, nil
}

// timeHandler writes an HTML page with the current time, news summary, and a meta refresh tag.
func timeHandler(w http.ResponseWriter, r *http.Request) {
	currentTime := time.Now().Format(time.RFC1123)

	// Get the latest news summary (from cache or fetch)
	summary, _ := getLatestNewsSummary() // Ignore error for display, error is part of the summary string if needed

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
`, currentTime, summary)
}

func main() {
	// Initial fetch of summary on startup (optional, can be blocking)
	// log.Println("Performing initial news summary fetch...")
	// getLatestNewsSummary() // Call once to populate cache initially

	// Register the timeHandler function for the root path.
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
