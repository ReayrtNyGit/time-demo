package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gomarkdown/markdown"
	mdhtml "github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
	"github.com/mmcdole/gofeed"
)

// Constants
const (
	maxItemsPerFeed = 3               // Number of items to display per feed
	cacheTTL        = 1 * time.Hour   // How long to cache the summary
	fetchTimeout    = 10 * time.Second // Timeout for fetching each feed
	serverPort      = ":8080"         // Port for the HTTP server
)

// Global variables for caching the news summary
var (
	cachedSummaryMD       string       // Cached summary in Markdown format
	cachedSummaryHTML     string       // Cached summary pre-rendered to HTML
	lastSummaryUpdateTime time.Time
	summaryMutex          sync.RWMutex // Read-write mutex for safe concurrent access
	summaryFetchError     error        // Store potential error during fetch
)

// Define the RSS feeds to fetch
var rssFeeds = []struct {
	Name string
	URL  string
}{
	{"BBC News", "http://feeds.bbci.co.uk/news/world/rss.xml"},
	// {"TechCrunch", "http://feeds.feedburner.com/TechCrunch/"}, // Removed TechCrunch
	{"The Guardian", "https://www.theguardian.com/world/rss"},
	{"NPR News", "https://feeds.npr.org/1001/rss.xml"},
	{"Al Jazeera", "http://www.aljazeera.com/xml/rss/all.xml"},
}

// fetchAndSummarizeNews fetches news from multiple RSS feeds concurrently
// and returns the combined summary in Markdown format.
func fetchAndSummarizeNews() (string, error) {
	// Configure HTTP client with timeout
	httpClient := &http.Client{Timeout: fetchTimeout}
	fp := gofeed.NewParser()
	fp.Client = httpClient // Assign the client to the parser

	var wg sync.WaitGroup
	var resultsMutex sync.Mutex
	results := make(map[string]string) // Store results keyed by feed name
	fetchErrors := []string{}          // Collect errors

	log.Printf("Fetching %d RSS feeds...", len(rssFeeds))

	for _, feedSource := range rssFeeds {
		wg.Add(1)
		go func(name, url string) {
			defer wg.Done()
			// Use context-aware parsing if needed, for now ParseURL with timeout client is sufficient
			feed, err := fp.ParseURL(url)
			if err != nil {
				log.Printf("Error fetching feed %s (%s): %v", name, url, err) // Use %v for errors
				resultsMutex.Lock()
				fetchErrors = append(fetchErrors, fmt.Sprintf("Failed to fetch %s: %v", name, err)) // Use %v
				resultsMutex.Unlock()
				return
			}

			var feedContent strings.Builder
			feedContent.WriteString(fmt.Sprintf("## %s\n\n", feed.Title)) // Use feed title from RSS

			count := 0
			for _, item := range feed.Items {
				if count >= maxItemsPerFeed {
					break
				}
				// Basic formatting: Title as link (if available)
				feedContent.WriteString(fmt.Sprintf("*   [%s](%s)\n", item.Title, item.Link))
				// Optionally add description:
				// feedContent.WriteString(fmt.Sprintf("    *Description:* %s\n", item.Description)) // Be mindful of HTML in descriptions
				count++
			}
			feedContent.WriteString("\n") // Add space after each feed section

			resultsMutex.Lock()
			results[name] = feedContent.String() // Store by original name for consistent ordering if needed
			resultsMutex.Unlock()
		}(feedSource.Name, feedSource.URL)
	}

	wg.Wait()
	log.Println("Finished fetching RSS feeds.")

	// Combine results - iterate through original list to maintain order
	var finalSummary strings.Builder
	for _, feedSource := range rssFeeds {
		if content, ok := results[feedSource.Name]; ok {
			finalSummary.WriteString(content)
		}
	}

	// Report any errors at the end
	if len(fetchErrors) > 0 {
		finalSummary.WriteString("\n---\n**Errors during fetch:**\n")
		for _, errMsg := range fetchErrors {
			finalSummary.WriteString(fmt.Sprintf("*   %s\n", errMsg))
		}
	}

	if finalSummary.Len() == 0 && len(fetchErrors) > 0 {
		// If all feeds failed
		return "", fmt.Errorf("failed to fetch any RSS feeds")
	}

	return finalSummary.String(), nil // Return combined summary, error is handled within the summary string or if all fail
}

// getLatestNewsSummary returns the cached summary (Markdown and HTML) if it's recent,
// otherwise triggers a new fetch.
func getLatestNewsSummary() (string, string, error) {
	summaryMutex.RLock() // Acquire read lock to check time
	// Use >= cacheTTL for comparison
	needsUpdate := time.Since(lastSummaryUpdateTime) >= cacheTTL || cachedSummaryMD == ""
	summaryMutex.RUnlock() // Release read lock

	if needsUpdate {
		summaryMutex.Lock() // Acquire write lock for potential update
		// Double-check if another goroutine updated it while waiting for the lock
		if time.Since(lastSummaryUpdateTime) >= cacheTTL || cachedSummaryMD == "" {
			log.Println("News summary cache expired or empty. Fetching new summary...")
			summaryMD, err := fetchAndSummarizeNews()
			if err != nil {
				log.Printf("Error fetching news summary: %v", err)
				// Keep the stale cache but store the error
				summaryFetchError = err
				// Optionally, clear the cache on error:
				// cachedSummaryMD = ""
				// cachedSummaryHTML = ""
			} else {
				// Convert Markdown to HTML here, only on successful fetch
				extensions := parser.CommonExtensions | parser.AutoHeadingIDs
				p := parser.NewWithExtensions(extensions)
				doc := p.Parse([]byte(summaryMD))
				htmlFlags := mdhtml.CommonFlags | mdhtml.HrefTargetBlank
				opts := mdhtml.RendererOptions{Flags: htmlFlags}
				renderer := mdhtml.NewRenderer(opts)
				summaryHTML := string(markdown.Render(doc, renderer))

				// Update cache
				cachedSummaryMD = summaryMD
				cachedSummaryHTML = summaryHTML
				summaryFetchError = nil // Clear previous error on success
			}
			lastSummaryUpdateTime = time.Now() // Update time even if fetch failed to prevent constant retries
		}
		summaryMutex.Unlock() // Release write lock
	}

	// Return the current cache content and any stored error
	summaryMutex.RLock()
	defer summaryMutex.RUnlock()
	// If there was an error during the last fetch attempt, return it along with potentially stale data
	if summaryFetchError != nil {
		// Return stale data but also the error
		return cachedSummaryMD, cachedSummaryHTML, summaryFetchError
	}
	// Return fresh (or acceptably old) data
	return cachedSummaryMD, cachedSummaryHTML, nil
}

// timeHandler writes an HTML page with the current time, news summary.
func timeHandler(w http.ResponseWriter, r *http.Request) {
	// Format the time as "Friday 18 April at 15:41"
	// Go's reference time: Mon Jan 2 15:04:05 MST 2006
	currentTime := time.Now().Format("Monday 02 January at 15:04")

	// Get the latest news summary (from cache or fetch)
	_, summaryHTML, err := getLatestNewsSummary() // We only need HTML for display
	if err != nil {
		// Log the error that occurred during the fetch/cache retrieval
		log.Printf("Handler warning: serving potentially stale news summary due to error: %v", err)
		// We still proceed to show potentially stale content, but the error is logged.
		// If summaryHTML is empty (e.g., first run failed), we might want to display an error message.
		if summaryHTML == "" {
			summaryHTML = "<p><em>Could not retrieve news summary. Please try again later.</em></p>"
		}
	}

	// Set headers
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Allow browser caching for 1 second to reduce rapid refresh load
	w.Header().Set("Cache-Control", "max-age=1")

	// Write the HTML response
	fmt.Fprintf(w, `
<!DOCTYPE html>
<html>
<head>
    <title>Current Time & News</title>
    <!-- Removed meta refresh -->
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif, "Apple Color Emoji", "Segoe UI Emoji", "Segoe UI Symbol";
            line-height: 1.6;
            margin: 0;
            padding: 20px;
            /* Dark mode background and text */
            background-color: #121212;
            color: #e0e0e0;
        }
        .container {
            max-width: 800px;
            margin: 20px auto;
            padding: 30px;
            /* Darker container background */
            background-color: #1e1e1e;
            border-radius: 8px;
            /* Subtle shadow for dark mode */
            box-shadow: 0 2px 10px rgba(0, 0, 0, 0.5);
        }
        h1 {
            /* Lighter heading color */
            color: #bb86fc;
            font-size: 1.8em;
            margin-bottom: 0.5em;
        }
        h2 {
            /* Lighter subheading color */
            color: #03dac6;
            font-size: 1.4em;
            margin-top: 1.5em;
            margin-bottom: 0.7em;
            /* Darker border */
            border-bottom: 1px solid #333;
            padding-bottom: 0.3em;
        }
        hr {
            border: 0;
            height: 1px;
            /* Darker separator */
            background-color: #444;
            margin: 2em 0;
        }
        pre { /* Style for potential future <pre> tags, not currently used by markdown output */
            white-space: pre-wrap;
            word-wrap: break-word;
            overflow-wrap: break-word;
            /* Darker code block background */
            background-color: #2c2c2c;
            padding: 15px;
            border-radius: 4px;
            font-family: "Courier New", Courier, monospace;
            font-size: 0.95em;
            /* Lighter code text */
            color: #e0e0e0;
        }
        /* Style links */
        a {
            color: #bb86fc; /* Link color matching h1 */
            text-decoration: none;
        }
        a:hover {
            text-decoration: underline;
        }
        /* Style paragraphs generated by Markdown */
        .container div p {
             margin-bottom: 1em; /* Add some space between paragraphs */
        }
        /* Style lists generated by Markdown */
        .container div ul {
            list-style: disc;
            margin-left: 20px;
            margin-bottom: 1em;
            /* Ensure list text color is light */
            color: #e0e0e0;
        }
        .container div li {
            margin-bottom: 0.5em;
        }
        /* Ensure list item links are styled correctly */
        .container div li a {
             color: #bb86fc;
        }
        .container div li a:hover {
             text-decoration: underline;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>The News on %s</h1> <!-- Updated heading text -->
        <hr>
        <h2>News Summary:</h2>
        <div>%s</div> <!-- Use a div for the pre-rendered HTML -->
    </div>
</body>
</html>
`, currentTime, summaryHTML) // Use the pre-rendered HTML summary
}

func main() {
	// Initial fetch of summary on startup (optional, can be blocking)
	// log.Println("Performing initial news summary fetch...")
	// getLatestNewsSummary() // Call once to populate cache initially

	// Register the timeHandler function for the root path.
	// Register the timeHandler function for the root path.
	http.HandleFunc("/", timeHandler)

	// Use the constant for the port
	log.Printf("Server starting on port %s\n", serverPort)

	// Start the HTTP server.
	// http.ListenAndServe always returns a non-nil error.
	if err := http.ListenAndServe(serverPort, nil); err != nil {
		log.Fatalf("Could not start server: %v\n", err) // Use %v for errors
	}
}
