//go:build headless

package main

import (
	"log"
	"os"
	"path/filepath"
)

func main() {
	if filepath.Base(os.Args[0]) == "axi-daemon" || len(os.Args) == 2 && os.Args[1] == "web" {
		if err := runWeb(); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := launchAxi(); err != nil {
		log.Fatal(err)
	}
}
