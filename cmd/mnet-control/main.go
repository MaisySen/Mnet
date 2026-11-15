// mnet-control is the Mnet control plane: registration,
// network maps, admin dashboard and the packet relay in one binary.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"time"

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
		oidcIss  = flag.String("oidc-issuer", os.Getenv("MNET_OIDC_ISSUER"), "OIDC issuer URL for SSO joins (or MNET_OIDC_ISSUER)")
		oidcID   = flag.String("oidc-client-id", os.Getenv("MNET_OIDC_CLIENT_ID"), "OIDC client id (or MNET_OIDC_CLIENT_ID)")
		oidcSec  = flag.String("oidc-client-secret", os.Getenv("MNET_OIDC_CLIENT_SECRET"), "OIDC client secret (or MNET_OIDC_CLIENT_SECRET)")
		showVer  = flag.Bool("version", false, "print version and exit")
		defNet   = flag.String("default-network", os.Getenv("MNET_DEFAULT_NETWORK"), "create this network when the store starts empty (or MNET_DEFAULT_NETWORK); survives hosts with ephemeral disks")
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
	if *defNet != "" && len(store.Networks()) == 0 {
		if _, err := store.CreateNetwork(*defNet); err != nil {
			log.Fatalf("default network: %v", err)
		}
		log.Printf("created default network %q (fresh state)", *defNet)
	}
	if *oidcIss != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		p, err := control.NewOIDCProvider(ctx, control.OIDCConfig{
			Issuer: *oidcIss, ClientID: *oidcID, ClientSecret: *oidcSec})
		cancel()
		if err != nil {
			log.Fatalf("oidc: %v", err)
		}
		srv.SetOIDC(p)
		log.Printf("sso: OIDC login via %s; register redirect URL <public base>/oidc/callback", *oidcIss)
	}
	if generated != "" {
		log.Printf("admin password (set MNET_ADMIN_PASSWORD to choose your own): %s", generated)
	}
	log.Printf("mnet-control %s listening on %s", version.Version, *listen)
	log.Printf("dashboard: %s/admin", publicBase)
	log.Fatal(http.ListenAndServe(*listen, srv.Handler()))
}
