package main

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
)

func runWeb() error {
	targetValue := strings.TrimRight(os.Getenv("AXI_AXIS_URL"), "/")
	if targetValue == "" {
		return errors.New("AXI_AXIS_URL is required")
	}
	target, err := url.Parse(targetValue)
	if err != nil {
		return fmt.Errorf("invalid AXI_AXIS_URL: %w", err)
	}
	frontend, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		return err
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director
	proxy.Director = func(request *http.Request) {
		originalDirector(request)
		username := os.Getenv("AXI_AXIS_USERNAME")
		password := os.Getenv("AXI_AXIS_PASSWORD")
		if username != "" || password != "" {
			request.SetBasicAuth(username, password)
		}
	}
	files := http.FileServer(http.FS(frontend))
	mux := http.NewServeMux()
	mux.Handle("/api/", proxy)
	mux.Handle("/runs/", proxy)
	mux.Handle("/sessions/", proxy)
	mux.HandleFunc("/", func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/" {
			if _, err := fs.Stat(frontend, strings.TrimPrefix(request.URL.Path, "/")); err == nil {
				files.ServeHTTP(response, request)
				return
			}
		}
		request.URL.Path = "/"
		files.ServeHTTP(response, request)
	})
	address := os.Getenv("AXI_WEB_ADDRESS")
	if address == "" {
		address = "127.0.0.1:8080"
	}
	log.Printf("serving Axi on http://%s", address)
	return http.ListenAndServe(address, mux)
}
