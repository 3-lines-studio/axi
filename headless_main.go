//go:build headless

package main

import (
	"fmt"
	"log"
	"os"
)

func main() {
	command := "web"
	if len(os.Args) == 2 {
		command = os.Args[1]
	} else if len(os.Args) > 2 {
		fmt.Fprintln(os.Stderr, "usage: axi [web|doctor]")
		os.Exit(2)
	}
	switch command {
	case "web":
		if err := runWeb(); err != nil {
			log.Fatal(err)
		}
	case "doctor":
		if err := runDoctor(); err != nil {
			log.Fatal(err)
		}
	default:
		fmt.Fprintln(os.Stderr, "usage: axi [web|doctor]")
		os.Exit(2)
	}
}
