//go:build linux || darwin

package main

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

func launchAxi() error {
	address := os.Getenv("AXI_WEB_ADDRESS")
	if address == "" {
		address = "127.0.0.1:8080"
	}
	browserURL, err := localBrowserURL(address)
	if err != nil {
		return err
	}
	running, occupied := probeAxi(browserURL)
	if running {
		return openBrowser(browserURL)
	}
	if occupied {
		return fmt.Errorf("%s is already used by another service", address)
	}
	logPath, err := startAxiDaemon()
	if err != nil {
		return err
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		running, occupied = probeAxi(browserURL)
		if running {
			return openBrowser(browserURL)
		}
		if occupied {
			return fmt.Errorf("%s was claimed by another service; see %s", address, logPath)
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("Axi did not start; see %s", logPath)
}

func startAxiDaemon() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	home, err := axiHome()
	if err != nil {
		return "", err
	}
	logDirectory := filepath.Join(home, "logs")
	if err := os.MkdirAll(logDirectory, 0700); err != nil {
		return "", err
	}
	logPath := filepath.Join(logDirectory, "axi.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return "", err
	}
	defer logFile.Close()
	null, err := os.Open(os.DevNull)
	if err != nil {
		return "", err
	}
	defer null.Close()
	command := exec.Command(executable)
	command.Args[0] = "axi-daemon"
	command.Stdin = null
	command.Stdout = logFile
	command.Stderr = logFile
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := command.Start(); err != nil {
		return "", err
	}
	if err := command.Process.Release(); err != nil {
		return "", err
	}
	return logPath, nil
}

func localBrowserURL(address string) (string, error) {
	target, err := url.Parse("http://" + address)
	if err != nil {
		return "", err
	}
	port := target.Port()
	if port == "" {
		return "", errors.New("AXI_WEB_ADDRESS must include a port")
	}
	return "http://127.0.0.1:" + port, nil
}

func probeAxi(address string) (bool, bool) {
	client := http.Client{Timeout: 300 * time.Millisecond}
	response, err := client.Get(strings.TrimRight(address, "/") + "/health")
	if err != nil {
		return false, false
	}
	response.Body.Close()
	return response.StatusCode == http.StatusNoContent && response.Header.Get("X-Axi-Service") == "1", true
}

func openBrowser(address string) error {
	var command *exec.Cmd
	if runtime.GOOS == "darwin" {
		command = exec.Command("open", address)
	} else {
		command = exec.Command("xdg-open", address)
	}
	if err := command.Start(); err != nil {
		fmt.Println(address)
		return nil
	}
	go command.Wait()
	return nil
}
