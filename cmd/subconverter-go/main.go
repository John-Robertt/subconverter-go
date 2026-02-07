package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/John-Robertt/subconverter-go/internal/httpapi"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:25500", "HTTP listen address")
	flag.Parse()

	srv := &http.Server{
		Addr:              *listen,
		Handler:           httpapi.NewHandler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("listening on http://%s", *listen)
	log.Fatal(srv.ListenAndServe())
}
