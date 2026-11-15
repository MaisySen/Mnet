// mnet-control is the Mnet control plane: registration,
// network maps, admin dashboard and the packet relay in one binary.
package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"fintex.au/mnet/internal/control"
	"fintex.au/mnet/internal/relay"
	"fintex.au/mnet/internal/version"
)

func main() {
	var (
		listen   = flag.String("listen", ":8080", "address to listen on")
		data     = flag.String("data", "mnet-control.json", "state file")
		base     = flag.String("base-url", "", "public base URL clients use, for example https://control.example.com (defaults to http://<listen>)")
		password = flag.String("admin-password", os.Getenv("MNET_ADMIN_PASSWORD"), "admin dashboard password (or MNET_ADMIN_PASSWORD); generated and printed if empty")
		showVer  = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()
	if *showVer {
		log.Printf("mnet-control %s", version.Version)
		return
	}

	store, err := control.Open(*data)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	hub := relay.NewHub()
	publicBase := *base
	if publicBase == "" {
		publicBase = "http://localhost" + *listen
	}
	srv, generated, err := control.NewServer(store, hub, publicBase, *password)
	if err != nil {
		log.Fatalf("server: %v", err)
	}
	if generated != "" {
		log.Printf("admin password (set MNET_ADMIN_PASSWORD to choose your own): %s", generated)
	}
	log.Printf("mnet-control %s listening on %s", version.Version, *listen)
	log.Printf("dashboard: %s/admin", publicBase)
	log.Fatal(http.ListenAndServe(*listen, srv.Handler()))
}
