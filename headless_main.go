//go:build headless

package main

import (
	"fmt"
	"log"
	"os"
)

func main() {
	if len(os.Args) != 2 || os.Args[1] != "web" {
		fmt.Fprintln(os.Stderr, "usage: axi web")
		os.Exit(2)
	}
	if err := runWeb(); err != nil {
		log.Fatal(err)
	}
}
