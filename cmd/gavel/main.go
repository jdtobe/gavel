package main

import (
	"fmt"

	"github.com/jdtobe/gavel/internal/version"
)

func main() {
	fmt.Printf("gavel %s\n", version.Version)
	fmt.Println("TUI and browser console are not implemented yet; see docs/superpowers/specs/2026-11-02-kev-playground-design.md")
}
