package main

// Version identifies the Resurrector core build. It is overridden at build
// time via -ldflags "-X main.Version=...". When running via `go run` or an
// unflagged build, it stays as "dev" so operators can tell the binary apart
// from a released artifact.
var Version = "dev"
