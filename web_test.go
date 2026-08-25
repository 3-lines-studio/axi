package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestBootstrapLocalConfig(t *testing.T) {
	home := t.TempDir()
	workingDirectory := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(workingDirectory); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		os.Chdir(previous)
	})
	config := filepath.Join(home, "config")
	if err := os.MkdirAll(config, 0700); err != nil {
		t.Fatal(err)
	}
	if err := bootstrapLocalConfig(home, config); err != nil {
		t.Fatal(err)
	}
	projectsData, err := os.ReadFile(filepath.Join(config, "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	var projects struct {
		Projects []struct {
			Path string `json:"path"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(projectsData, &projects); err != nil {
		t.Fatal(err)
	}
	if len(projects.Projects) != 1 || projects.Projects[0].Path != workingDirectory {
		t.Fatalf("unexpected projects: %+v", projects.Projects)
	}
	botsData, err := os.ReadFile(filepath.Join(config, "bots.json"))
	if err != nil {
		t.Fatal(err)
	}
	var bots struct {
		Bots []struct {
			ID            string `json:"id"`
			WorkspaceRoot string `json:"workspace_root"`
		} `json:"bots"`
	}
	if err := json.Unmarshal(botsData, &bots); err != nil {
		t.Fatal(err)
	}
	if len(bots.Bots) != 1 || bots.Bots[0].ID != "assistant" {
		t.Fatalf("unexpected bots: %+v", bots.Bots)
	}
	if bots.Bots[0].WorkspaceRoot != filepath.Join(home, "workspaces", "assistant") {
		t.Fatalf("unexpected workspace: %s", bots.Bots[0].WorkspaceRoot)
	}
	info, err := os.Stat(filepath.Join(config, "bots.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("unexpected mode: %o", info.Mode().Perm())
	}
}

func TestLocalSetup(t *testing.T) {
	config := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config)
	t.Setenv("OPENAI_API_KEY", "")

	request := httptest.NewRequest(http.MethodGet, "/api/setup", nil)
	response := httptest.NewRecorder()
	getLocalSetup(response, request)
	if response.Code != http.StatusOK || response.Body.String() != "{\"configured\":false}\n" {
		t.Fatalf("unexpected status: %d %s", response.Code, response.Body.String())
	}

	body := bytes.NewBufferString(`{"api_key":"secret","model":"model","base":"https://example.com/v1/"}`)
	request = httptest.NewRequest(http.MethodPost, "/api/setup", body)
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	saveLocalSetup(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("unexpected status: %d %s", response.Code, response.Body.String())
	}
	data, err := os.ReadFile(filepath.Join(config, "ax", "config"))
	if err != nil {
		t.Fatal(err)
	}
	want := "api_key = \"secret\"\nmodel = \"model\"\nbase = \"https://example.com/v1\"\n"
	if string(data) != want {
		t.Fatalf("unexpected config: %s", data)
	}
	info, err := os.Stat(filepath.Join(config, "ax", "config"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("unexpected mode: %o", info.Mode().Perm())
	}
	if err := os.WriteFile(filepath.Join(config, "ax", "config"), append(data, []byte("context_window = 1000\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	body = bytes.NewBufferString(`{"api_key":"","model":"other-model","base":""}`)
	request = httptest.NewRequest(http.MethodPost, "/api/setup", body)
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	saveLocalSetup(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("unexpected status: %d %s", response.Code, response.Body.String())
	}
	data, err = os.ReadFile(filepath.Join(config, "ax", "config"))
	if err != nil {
		t.Fatal(err)
	}
	want = "api_key = \"secret\"\nmodel = \"other-model\"\ncontext_window = 1000\n"
	if string(data) != want {
		t.Fatalf("unexpected updated config: %s", data)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/setup", nil)
	response = httptest.NewRecorder()
	getLocalSetup(response, request)
	if response.Code != http.StatusOK || response.Body.String() != "{\"configured\":true}\n" {
		t.Fatalf("unexpected status: %d %s", response.Code, response.Body.String())
	}
}

func TestValidateWebAddress(t *testing.T) {
	for _, address := range []string{"127.0.0.1:8080", "localhost:8080", "[::1]:8080"} {
		if err := validateWebAddress(address); err != nil {
			t.Fatalf("%s: %v", address, err)
		}
	}
	if err := validateWebAddress("0.0.0.0:8080"); err == nil {
		t.Fatal("public address accepted")
	}
	t.Setenv("AXI_ALLOW_PUBLIC", "true")
	if err := validateWebAddress("0.0.0.0:8080"); err != nil {
		t.Fatal(err)
	}
}

func TestBootstrapLocalConfigPreservesExistingConfig(t *testing.T) {
	home := t.TempDir()
	config := filepath.Join(home, "config")
	if err := os.MkdirAll(config, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(config, "projects.json")
	if err := os.WriteFile(path, []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := bootstrapLocalConfig(home, config); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "existing" {
		t.Fatalf("config changed: %s", data)
	}
	if _, err := os.Stat(filepath.Join(config, "bots.json")); !os.IsNotExist(err) {
		t.Fatal("bots config was created")
	}
}
