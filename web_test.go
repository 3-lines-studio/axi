package main

import (
	"encoding/json"
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
