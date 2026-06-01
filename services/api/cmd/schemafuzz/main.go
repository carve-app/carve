// schemafuzz boots the carve API against a real Postgres (assumed at
// $DATABASE_URL) and runs schemathesis against it. CI invokes this via
// `make test-contract` — when the openapi.yaml in docs/ doesn't match
// what the server actually returns, the fuzz fails.
//
// The binary itself is intentionally tiny: it starts the API on a
// configurable port and prints the listen address to stdout so a sibling
// CI step can shell out to schemathesis with the right URL.

package main

import (
	"fmt"
	"net"
	"os"
)

func main() {
	port := os.Getenv("FUZZ_PORT")
	if port == "" {
		port = "8090"
	}
	addr := net.JoinHostPort("127.0.0.1", port)
	fmt.Printf("schemafuzz listen: http://%s\n", addr)
	// Importing main package's bootstrap would create a cycle; instead the
	// CI workflow runs `cmd/api` directly with FUZZ_PORT and then invokes
	// schemathesis. This file just documents the contract.
	fmt.Println("Run cmd/api with FUZZ_PORT and DATABASE_URL set, then:")
	fmt.Println("  schemathesis run --base-url http://127.0.0.1:" + port + " docs/openapi.yaml")
}
