package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type localAxis struct {
	command  *exec.Cmd
	username string
	password string
	url      string
	done     chan error
}

func runWeb() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	targetValue := strings.TrimRight(os.Getenv("AXI_AXIS_URL"), "/")
	username := os.Getenv("AXI_AXIS_USERNAME")
	password := os.Getenv("AXI_AXIS_PASSWORD")
	var local *localAxis
	var err error
	if targetValue == "" {
		local, err = startLocalAxis(ctx)
		if err != nil {
			return err
		}
		targetValue = local.url
		username = local.username
		password = local.password
		defer local.stop()
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
		if username != "" || password != "" {
			request.SetBasicAuth(username, password)
		}
	}
	files := http.FileServer(http.FS(frontend))
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(response http.ResponseWriter, request *http.Request) {
		response.WriteHeader(http.StatusNoContent)
	})
	if local != nil {
		mux.HandleFunc("GET /api/setup", getLocalSetup)
		mux.HandleFunc("POST /api/setup", saveLocalSetup)
	}
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
	if err := validateWebAddress(address); err != nil {
		return err
	}
	server := &http.Server{Addr: address, Handler: mux}
	serverError := make(chan error, 1)
	go func() {
		log.Printf("serving Axi on http://%s", address)
		serverError <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	case err := <-serverError:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case err := <-localDone(local):
		return fmt.Errorf("local Axis stopped: %w", err)
	}
}

func startLocalAxis(ctx context.Context) (*localAxis, error) {
	axisPath, err := exec.LookPath("axis")
	if err != nil {
		return nil, errors.New("local Axis is not installed; install axis or set AXI_AXIS_URL")
	}
	if _, err := exec.LookPath("ax"); err != nil {
		return nil, errors.New("AX is not installed")
	}
	home, err := axiHome()
	if err != nil {
		return nil, err
	}
	config := filepath.Join(home, "config")
	if err := os.MkdirAll(filepath.Join(config, "bots"), 0700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(config, "connectors"), 0700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(home, "data"), 0700); err != nil {
		return nil, err
	}
	if err := bootstrapLocalConfig(home, config); err != nil {
		return nil, err
	}
	address, err := availableAddress()
	if err != nil {
		return nil, err
	}
	username, err := randomCredential()
	if err != nil {
		return nil, err
	}
	password, err := randomCredential()
	if err != nil {
		return nil, err
	}
	command := exec.CommandContext(ctx, axisPath)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Env = append(os.Environ(),
		"AXIS_ADDRESS="+address,
		"AXIS_USERNAME="+username,
		"AXIS_PASSWORD="+password,
		"AXIS_DATA_DIR="+filepath.Join(home, "data"),
		"AXIS_PROJECTS_FILE="+filepath.Join(config, "projects.json"),
		"AXIS_BOTS_FILE="+filepath.Join(config, "bots.json"),
		"AXIS_CONNECTORS_FILE="+filepath.Join(config, "connectors.json"),
		"AXIS_BOT_ENV_DIR="+filepath.Join(config, "bots"),
		"AXIS_CONNECTOR_ENV_DIR="+filepath.Join(config, "connectors"),
		"AXIS_CONNECTOR_URL=http://"+address,
	)
	if os.Getenv("AX_TOOLS") == "" {
		command.Env = append(command.Env, "AX_TOOLS=fsx bashx skillx attachx")
	}
	if err := command.Start(); err != nil {
		return nil, err
	}
	local := &localAxis{
		command:  command,
		username: username,
		password: password,
		url:      "http://" + address,
		done:     make(chan error, 1),
	}
	go func() {
		local.done <- command.Wait()
	}()
	if err := waitForAxis(ctx, address, local.done); err != nil {
		local.stop()
		return nil, err
	}
	log.Printf("started local Axis with home %s", home)
	return local, nil
}

func bootstrapLocalConfig(home string, config string) error {
	projectsPath := filepath.Join(config, "projects.json")
	botsPath := filepath.Join(config, "bots.json")
	if _, err := os.Stat(projectsPath); !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if _, err := os.Stat(botsPath); !errors.Is(err, os.ErrNotExist) {
		return err
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return err
	}
	workspace := filepath.Join(home, "workspaces", "assistant")
	if err := os.MkdirAll(workspace, 0700); err != nil {
		return err
	}
	projects := map[string]any{
		"roots": []map[string]string{
			{"name": "Current directory", "path": workingDirectory},
			{"name": "Axi workspaces", "path": filepath.Join(home, "workspaces")},
		},
		"projects": []map[string]string{
			{"id": "default", "name": filepath.Base(workingDirectory), "path": workingDirectory},
		},
	}
	tools := strings.Fields(os.Getenv("AX_TOOLS"))
	if len(tools) == 0 {
		tools = []string{"fsx", "bashx", "skillx", "attachx"}
	}
	bots := map[string]any{
		"bots": []map[string]any{
			{
				"id":             "assistant",
				"name":           "Assistant",
				"prompt":         "Help the user with tasks in the selected project.",
				"tools":          tools,
				"workspace_root": workspace,
			},
		},
	}
	if err := writePrivateJSON(projectsPath, projects); err != nil {
		return err
	}
	return writePrivateJSON(botsPath, bots)
}

func writePrivateJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writePrivateFile(path, append(data, '\n'))
}

type localSetup struct {
	APIKey string `json:"api_key"`
	Model  string `json:"model"`
	Base   string `json:"base"`
}

func getLocalSetup(response http.ResponseWriter, request *http.Request) {
	configured, err := providerConfigured()
	if err != nil {
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}
	response.Header().Set("Content-Type", "application/json")
	json.NewEncoder(response).Encode(map[string]bool{"configured": configured})
}

func providerConfigured() (bool, error) {
	if os.Getenv("OPENAI_API_KEY") != "" {
		return true, nil
	}
	data, err := os.ReadFile(axConfigPath())
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if found && (strings.TrimSpace(key) == "api_key" || strings.TrimSpace(key) == "base") && strings.TrimSpace(value) != "" {
			return true, nil
		}
	}
	return false, nil
}

func saveLocalSetup(response http.ResponseWriter, request *http.Request) {
	if request.Header.Get("Content-Type") != "application/json" {
		http.Error(response, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
		return
	}
	request.Body = http.MaxBytesReader(response, request.Body, 64<<10)
	var setup localSetup
	if err := json.NewDecoder(request.Body).Decode(&setup); err != nil {
		http.Error(response, "invalid setup", http.StatusBadRequest)
		return
	}
	setup.APIKey = strings.TrimSpace(setup.APIKey)
	setup.Model = strings.TrimSpace(setup.Model)
	setup.Base = strings.TrimRight(strings.TrimSpace(setup.Base), "/")
	path := axConfigPath()
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}
	var apiKeyLine string
	var otherLines []string
	for _, line := range strings.Split(strings.TrimRight(string(existing), "\n"), "\n") {
		key, _, found := strings.Cut(line, "=")
		switch strings.TrimSpace(key) {
		case "api_key":
			apiKeyLine = line
		case "model", "base":
		default:
			if found || strings.TrimSpace(line) != "" {
				otherLines = append(otherLines, line)
			}
		}
	}
	if setup.APIKey != "" {
		apiKeyLine = "api_key = " + strconv.Quote(setup.APIKey)
	}
	if apiKeyLine == "" && setup.Base == "" {
		http.Error(response, "API key or base URL is required", http.StatusBadRequest)
		return
	}
	var lines []string
	if apiKeyLine != "" {
		lines = append(lines, apiKeyLine)
	}
	if setup.Model != "" {
		lines = append(lines, "model = "+strconv.Quote(setup.Model))
	}
	if setup.Base != "" {
		lines = append(lines, "base = "+strconv.Quote(setup.Base))
	}
	lines = append(lines, otherLines...)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := writePrivateFile(path, []byte(strings.Join(lines, "\n")+"\n")); err != nil {
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}
	response.WriteHeader(http.StatusNoContent)
}

func axConfigPath() string {
	config := os.Getenv("XDG_CONFIG_HOME")
	if config == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			config = filepath.Join(home, ".config")
		}
	}
	return filepath.Join(config, "ax", "config")
}

func writePrivateFile(path string, data []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".axi-*")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(0600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func validateWebAddress(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid AXI_WEB_ADDRESS: %w", err)
	}
	if host == "localhost" || net.ParseIP(host).IsLoopback() {
		return nil
	}
	if os.Getenv("AXI_ALLOW_PUBLIC") == "true" {
		return nil
	}
	return errors.New("public Axi Web requires AXI_ALLOW_PUBLIC=true and an authentication proxy")
}

func availableAddress() (string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		return "", err
	}
	return address, nil
}

func randomCredential() (string, error) {
	value := make([]byte, 24)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func waitForAxis(ctx context.Context, address string, done <-chan error) error {
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-done:
			return fmt.Errorf("Axis failed to start: %w", err)
		case <-timeout.C:
			return errors.New("Axis did not start within 5 seconds")
		case <-ticker.C:
			connection, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
			if err == nil {
				connection.Close()
				return nil
			}
		}
	}
}

func localDone(local *localAxis) <-chan error {
	if local == nil {
		return make(chan error)
	}
	return local.done
}

func (local *localAxis) stop() {
	if local == nil || local.command.Process == nil || local.command.ProcessState != nil {
		return
	}
	local.command.Process.Signal(syscall.SIGTERM)
	select {
	case <-local.done:
	case <-time.After(5 * time.Second):
		local.command.Process.Kill()
	}
}
