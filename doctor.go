package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func runDoctor() error {
	failed := false
	for _, binary := range []string{"axis", "ax", "fsx", "bashx", "skillx", "attachx"} {
		path, err := exec.LookPath(binary)
		if err != nil {
			fmt.Printf("missing  %s\n", binary)
			failed = true
			continue
		}
		fmt.Printf("ok       %s %s\n", binary, path)
	}
	home, err := axiHome()
	if err != nil {
		fmt.Printf("failed   Axi home: %v\n", err)
		failed = true
	} else if err := writableDirectory(home); err != nil {
		fmt.Printf("failed   Axi home %s: %v\n", home, err)
		failed = true
	} else {
		fmt.Printf("ok       Axi home %s\n", home)
	}
	configured, err := providerConfigured()
	if err != nil {
		fmt.Printf("failed   AX provider: %v\n", err)
		failed = true
	} else if configured {
		fmt.Println("ok       AX provider configured")
	} else {
		fmt.Println("missing  AX provider; run axi web to complete setup")
		failed = true
	}
	if failed {
		return errors.New("Axi is not ready")
	}
	fmt.Println("ready    Axi can start")
	return nil
}

func axiHome() (string, error) {
	if home := strings.TrimSpace(os.Getenv("AXI_HOME")); home != "" {
		return filepath.Abs(home)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "axi"), nil
}

func writableDirectory(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(path, ".doctor-*")
	if err != nil {
		return err
	}
	name := file.Name()
	if err := file.Close(); err != nil {
		os.Remove(name)
		return err
	}
	return os.Remove(name)
}
