// Command demo serves the PicoPost development demo site.
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

func main() {
	addr := flag.String("listen", "127.0.0.1:8081", "listen address")
	dir := flag.String("dir", "", "web directory to serve (default: ./web)")
	flag.Parse()

	webDir := *dir
	if webDir == "" {
		webDir = "web"
	}
	abs, err := filepath.Abs(webDir)
	if err != nil {
		log.Fatal(err)
	}
	if _, err := os.Stat(abs); err != nil {
		log.Fatalf("web directory %s: %v", abs, err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir(abs)))

	log.Printf("demo site listening on %s (serving %s)", *addr, abs)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}
