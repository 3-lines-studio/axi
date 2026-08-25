//go:build headless

package main

import (
	"log"
	"os"
	"path/filepath"
)

func main() {
	if filepath.Base(os.Args[0]) == "axi-updater" {
		if len(os.Args) != 3 {
			log.Fatal("invalid update")
		}
		if err := applyUpdate(os.Args[1], os.Args[2]); err != nil {
			log.Print(err)
		}
		return
	}
	if filepath.Base(os.Args[0]) == "axi-daemon" {
		home, err := axiHome()
		if err != nil {
			log.Fatal(err)
		}
		logger, err := newRollingLog(home)
		if err != nil {
			log.Fatal(err)
		}
		defer logger.Close()
		log.SetOutput(logger)
		if err := runWeb(); err != nil {
			log.Print(err)
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "web" {
		if err := runWeb(); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := launchAxi(); err != nil {
		log.Fatal(err)
	}
}
