Below is a “shopping list” of things you could consider.  
None of them are strictly required – the program already works – but each addresses either performance, robustness, readability, or long‑term maintainability.

────────────────────────────────
1. Configurability / constants
────────────────────────────────
• Move “magic numbers” (maxItemsPerFeed, refresh interval, cache TTL, port, etc.) to a small Config struct that can be filled from flags or env‑vars (flag, github.com/kelseyhightower/envconfig, etc.).  
• Make the feed list part of that config so you can add / remove feeds without recompiling.

────────────────────────────────
2. Fetching feeds
────────────────────────────────
• Use a context‐aware HTTP client with a timeout:

    httpClient := &http.Client{Timeout: 10 * time.Second}
    fp := gofeed.NewParser()
    fp.Client = httpClient

This prevents a single hung endpoint from stalling the worker goroutine forever.

• Put the fetch in its own context so that shutting the server down cancels all in‑flight requests.

• Concurrency limiter: if you later grow the list to dozens of feeds, add a buffered channel (or errgroup.WithContext+semaphore) to cap the number of simultaneous dials.

────────────────────────────────
3. Error handling
────────────────────────────────
• At the call‑site of getLatestNewsSummary you currently drop the returned error.  
  Show it to the user or log it, otherwise the caller cannot tell “everything failed”.

• Instead of returning both a rendered summary and an error, return

    type Summary struct {
        Markdown string
        HTML     string
        Fresh    bool          // whether it was just fetched
        Err      error         // aggregate error
    }

  …so the handler can decide what to render.

• In Go 1.20+ you can use errors.Join to aggregate fetchErrors instead of strings.

────────────────────────────────
4. Caching
────────────────────────────────
• Convert the markdown to HTML once, when the cache is (re)built, and store both
  versions.   At the moment you parse+render the same markdown every second
  because of the meta‑refresh.

• Make the TTL configurable; for news, 5‑10 min is usually fine and reduces
  outbound traffic.

────────────────────────────────
5. HTML generation
────────────────────────────────
• Switch from fmt.Fprintf to html/template.  
  – It auto‑escapes variables.  
  – You can keep the markup in a separate file for cleaner code.  
  – You can gzip the response with a middleware (net/http’s built‑in
    gzip handler from Go 1.22 or github.com/nytimes/gziphandler).

• The heading “FT News Summary” looks copy‑pasted; make it dynamic or fix it.

────────────────────────────────
6. Code structure
────────────────────────────────
• Split the file:  
  – feeds.go        (fetching & summarisation)  
  – cache.go        (TTL cache wrapper)  
  – handler.go      (HTTP layer)  
  – main.go         (wiring / flags)

• Provide a graceful shutdown: create an http.Server, call server.Shutdown(ctx)
  on SIGINT/SIGTERM.

────────────────────────────────
7. Output quality
────────────────────────────────
• Sort items by publish date instead of “first N returned”; most feeds
  already include PublishedParsed.

• Guard against missing fields:

    title := strings.TrimSpace(item.Title)
    if title == "" { title = "Untitled article" }

• Feed.Title can be empty – fall back to the name from the config.

• Strip HTML tags from item.Description before writing it (html2text).

────────────────────────────────
8. Minor Go idioms
────────────────────────────────
• Prefer time.Since(last) >= cacheTTL over “>”.
• Use log.Printf("…: %v", err) not %s for errors that implement Error().
• Replace resultsMutex+map with sync.Map or a channel fan‑in if you like.
• Run go vet, go test ./..., golangci‑lint; they’ll point out small issues
  (e.g. unsynchronised access to fetchErrors slice if you ever read it outside
  the critical section).

────────────────────────────────
9. Security / abuse
────────────────────────────────
• The meta‑refresh every second causes a full HTTP request flood if many users
  open the page.  Consider websockets/SSE or a longer interval.  At minimum add
  Cache‑Control: max‑age=1 to allow the browser to reuse the last response for
  that one‑second period.

• Add a robots.txt disallow rule if the site is public; crawlers will happily
  follow a 1‑second refresh and create load.

────────────────────────────────
10. Testing
────────────────────────────────
• Wrap the fetcher behind an interface so you can inject a fake feed in unit
  tests without making real network calls.

• Table‑driven tests for markdown->HTML conversion and cache invalidation logic
  are easy to add and stop regressions.

────────────────────────────────
Quick example ­– pre‑render markdown:

    type cached struct {
        md       string
        html     string
        fetched  time.Time
        err      error
    }

    var news cached
    …
    news.md   = summaryMD
    news.html = string(markdown.ToHTML([]byte(summaryMD), nil, renderer))

and in the handler:

    summaryHTML := news.html   // no work on hot path

With these tweaks you’ll have a small, fast, and much easier‑to‑maintain news
aggregator.
