package main

import (
	"log"
	"net/http"

	"github.com/conray/mysqlweb/internal/api"
)

var version = "dev"

func main() {
	r := api.NewRouter(api.Deps{Version: version})
	addr := ":53306"
	log.Printf("mysqlweb listening on %s (version=%s)", addr, version)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatal(err)
	}
}
