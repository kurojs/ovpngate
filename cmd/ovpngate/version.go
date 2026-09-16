package main

// Version is injected at build time with:
//
//	go build -ldflags "-X main.Version=v0.2.2" ./cmd/ovpngate
//
// It falls back to "dev" for local builds.
var Version = "dev"
