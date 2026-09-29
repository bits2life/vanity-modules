// Command vanity is a standalone server for Go vanity import paths.
//
// It is configured through the environment; see the package documentation of
// go.bits2life.com/vanity-modules for the variables. The listen address is taken
// from VANITY_ADDR, then PORT, and defaults to :8080.
package main

import (
	"log"
	"net/http"
	"os"

	vanity "go.bits2life.com/vanity-modules"
)

func main() {
	cfg, err := vanity.FromEnv()
	if err != nil {
		log.Fatal(err)
	}
	h, err := vanity.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	addr := os.Getenv("VANITY_ADDR")
	if addr == "" {
		if port := os.Getenv("PORT"); port != "" {
			addr = ":" + port
		} else {
			addr = ":8080"
		}
	}
	log.Printf("vanity: listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, h))
}
