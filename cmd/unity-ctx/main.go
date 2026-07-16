package main

import (
	"os"

	"github.com/Kubonsang/unity-ctx/internal/cli"
	"github.com/Kubonsang/unity-ctx/internal/reviewbridge"
)

func main() {
	// The local human-review bridge uses a signed, one-request stdio protocol.
	// This command is intentionally absent from MCP/tools and the ordinary CLI
	// router; without a registered Ed25519 authority key and a fresh grant it
	// cannot approve or write a contract.
	if len(os.Args) == 2 && os.Args[1] == "review-bridge" {
		os.Exit(reviewbridge.Run(os.Stdin, os.Stdout, os.Stderr, reviewbridge.Config{}))
	}
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
