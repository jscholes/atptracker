package main

import (
	"embed"
	"fmt"
	"log"
	"net/http"
	"os"
)

//go:embed index.html
var staticFS embed.FS

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fs := http.FileServer(http.FS(staticFS))
	http.Handle("/", fs)

	log.Printf("Serving task creation interface on port %s; press Ctrl+C to quit", port)

	serveAddr := fmt.Sprintf(":%s", port)
	log.Fatal(http.ListenAndServe(serveAddr, nil))
}
