package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var latestReleaseURL = "https://api.github.com/repos/3-lines-studio/axi/releases/latest"

var bundleBinaries = []string{"axi", "axis", "ax", "fsx", "bashx", "skillx", "attachx"}

type updateStatus struct {
	Current     string `json:"current"`
	Latest      string `json:"latest,omitempty"`
	Available   bool   `json:"available"`
	Downloaded  bool   `json:"downloaded"`
	Checking    bool   `json:"checking"`
	Error       string `json:"error,omitempty"`
	LastChecked string `json:"last_checked,omitempty"`
}

type updater struct {
	home    string
	mu      sync.Mutex
	status  updateStatus
	staging string
}

type githubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

func newUpdater(home string) *updater {
	return &updater{home: home, status: updateStatus{Current: version}}
}

func (u *updater) currentStatus() updateStatus {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.status
}

func (u *updater) check(force bool) {
	if version == "dev" {
		return
	}
	marker := filepath.Join(u.home, "updates", "last-check")
	if !force {
		if info, err := os.Stat(marker); err == nil && time.Since(info.ModTime()) < 24*time.Hour {
			return
		}
	}
	u.mu.Lock()
	if u.status.Checking {
		u.mu.Unlock()
		return
	}
	u.status.Checking = true
	u.status.Error = ""
	u.mu.Unlock()

	err := u.checkAndDownload()
	now := time.Now().UTC()
	if mkdirErr := os.MkdirAll(filepath.Dir(marker), 0700); err == nil && mkdirErr != nil {
		err = mkdirErr
	}
	if err == nil {
		if writeErr := os.WriteFile(marker, []byte(now.Format(time.RFC3339)+"\n"), 0600); writeErr != nil {
			err = writeErr
		}
	}
	u.mu.Lock()
	u.status.Checking = false
	u.status.LastChecked = now.Format(time.RFC3339)
	if err != nil {
		u.status.Error = err.Error()
	}
	u.mu.Unlock()
}

func (u *updater) checkAndDownload() error {
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, latestReleaseURL, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "Axi/"+version)
	client := http.Client{Timeout: 30 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("update check returned %s", response.Status)
	}
	var release githubRelease
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&release); err != nil {
		return err
	}
	u.mu.Lock()
	u.status.Latest = release.TagName
	u.status.Available = newerVersion(release.TagName, version)
	u.status.Downloaded = false
	u.staging = ""
	available := u.status.Available
	u.mu.Unlock()
	if !available {
		return nil
	}
	target, err := updateTarget()
	if err != nil {
		return err
	}
	bundleName := "axi-bundle-" + target + ".tar.gz"
	checksumName := bundleName + ".sha256"
	var bundleURL string
	var checksumURL string
	for _, asset := range release.Assets {
		switch asset.Name {
		case bundleName:
			bundleURL = asset.URL
		case checksumName:
			checksumURL = asset.URL
		}
	}
	if bundleURL == "" || checksumURL == "" {
		return errors.New("release bundle is missing")
	}
	directory := filepath.Join(u.home, "updates", release.TagName)
	if err := os.RemoveAll(directory); err != nil {
		return err
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	bundlePath := filepath.Join(directory, bundleName)
	checksumPath := bundlePath + ".sha256"
	if err := downloadUpdate(client, bundleURL, bundlePath, 150<<20); err != nil {
		return err
	}
	if err := downloadUpdate(client, checksumURL, checksumPath, 4096); err != nil {
		return err
	}
	checksumData, err := os.ReadFile(checksumPath)
	if err != nil {
		return err
	}
	fields := strings.Fields(string(checksumData))
	if len(fields) == 0 {
		return errors.New("release checksum is empty")
	}
	if err := verifySHA256(bundlePath, fields[0]); err != nil {
		return err
	}
	staging := filepath.Join(directory, "staging")
	if err := extractBundle(bundlePath, staging); err != nil {
		return err
	}
	u.mu.Lock()
	u.status.Downloaded = true
	u.staging = staging
	u.mu.Unlock()
	return nil
}

func newerVersion(candidate string, current string) bool {
	parse := func(value string) ([3]int, bool) {
		var result [3]int
		parts := strings.Split(strings.TrimPrefix(value, "v"), ".")
		if len(parts) != 3 {
			return result, false
		}
		for index, part := range parts {
			number, err := strconv.Atoi(part)
			if err != nil {
				return result, false
			}
			result[index] = number
		}
		return result, true
	}
	next, nextOK := parse(candidate)
	installed, installedOK := parse(current)
	if !nextOK || !installedOK {
		return false
	}
	for index := range next {
		if next[index] != installed[index] {
			return next[index] > installed[index]
		}
	}
	return false
}

func updateTarget() (string, error) {
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x86_64"
	} else if arch == "arm64" {
		arch = "aarch64"
	} else {
		return "", fmt.Errorf("unsupported update architecture: %s", arch)
	}
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		return "", fmt.Errorf("unsupported update system: %s", runtime.GOOS)
	}
	return runtime.GOOS + "-" + arch, nil
}

func downloadUpdate(client http.Client, address string, path string, limit int64) error {
	response, err := client.Get(address)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("update download returned %s", response.Status)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(file, io.LimitReader(response.Body, limit+1))
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if written > limit {
		return errors.New("update download is too large")
	}
	return nil
}

func verifySHA256(path string, expected string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if actual != strings.ToLower(expected) {
		return errors.New("update checksum does not match")
	}
	return nil
}

func extractBundle(path string, destination string) error {
	if err := os.MkdirAll(destination, 0700); err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	compressed, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer compressed.Close()
	allowed := make(map[string]bool, len(bundleBinaries))
	for _, name := range bundleBinaries {
		allowed[name] = true
	}
	found := make(map[string]bool, len(bundleBinaries))
	archive := tar.NewReader(compressed)
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if !allowed[header.Name] || header.Typeflag != tar.TypeReg || header.Size <= 0 || header.Size > 100<<20 || found[header.Name] {
			return fmt.Errorf("invalid update entry: %s", header.Name)
		}
		output, err := os.OpenFile(filepath.Join(destination, header.Name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
		if err != nil {
			return err
		}
		_, copyErr := io.CopyN(output, archive, header.Size)
		closeErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		found[header.Name] = true
	}
	for _, name := range bundleBinaries {
		if !found[name] {
			return fmt.Errorf("update is missing %s", name)
		}
	}
	return nil
}

func (u *updater) startInstall(address string, logFile *os.File) error {
	u.mu.Lock()
	staging := u.staging
	ready := u.status.Downloaded
	u.mu.Unlock()
	if !ready || staging == "" {
		return errors.New("no update is ready")
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	command := exec.Command(executable, staging, address)
	command.Args[0] = "axi-updater"
	command.Stdin = nil
	command.Stdout = logFile
	command.Stderr = logFile
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := command.Start(); err != nil {
		return err
	}
	return command.Process.Release()
}

func applyUpdate(staging string, address string) error {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		_, occupied := probeAxi(address)
		if !occupied {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if _, occupied := probeAxi(address); occupied {
		return errors.New("Axi did not stop for update")
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	binaryDirectory := filepath.Dir(executable)
	home, err := axiHome()
	if err != nil {
		return err
	}
	backup := filepath.Join(home, "updates", "previous")
	if err := os.RemoveAll(backup); err != nil {
		return err
	}
	if err := os.MkdirAll(backup, 0700); err != nil {
		return err
	}
	for _, name := range bundleBinaries {
		if err := copyExecutable(filepath.Join(binaryDirectory, name), filepath.Join(backup, name)); err != nil {
			return err
		}
	}
	for _, name := range bundleBinaries {
		if err := replaceExecutable(filepath.Join(staging, name), filepath.Join(binaryDirectory, name)); err != nil {
			rollbackUpdate(backup, binaryDirectory)
			return err
		}
	}
	updated, err := startUpdatedAxi(filepath.Join(binaryDirectory, "axi"), home)
	if err != nil {
		rollbackUpdate(backup, binaryDirectory)
		return err
	}
	deadline = time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		running, _ := probeAxi(address)
		if running {
			updated.Process.Release()
			os.RemoveAll(staging)
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	updated.Process.Kill()
	updated.Wait()
	rollbackUpdate(backup, binaryDirectory)
	previous, err := startUpdatedAxi(filepath.Join(binaryDirectory, "axi"), home)
	if err != nil {
		return fmt.Errorf("update and rollback restart failed: %w", err)
	}
	previous.Process.Release()
	return errors.New("updated Axi failed health check; previous version restored")
}

func copyExecutable(source string, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func replaceExecutable(source string, destination string) error {
	staged := filepath.Join(filepath.Dir(destination), "."+filepath.Base(destination)+".update")
	os.Remove(staged)
	if err := copyExecutable(source, staged); err != nil {
		return err
	}
	return os.Rename(staged, destination)
}

func rollbackUpdate(backup string, binaryDirectory string) {
	for _, name := range bundleBinaries {
		replaceExecutable(filepath.Join(backup, name), filepath.Join(binaryDirectory, name))
	}
}

func startUpdatedAxi(executable string, home string) (*exec.Cmd, error) {
	logFile, err := os.OpenFile(filepath.Join(home, "logs", "axi.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, err
	}
	defer logFile.Close()
	command := exec.Command(executable)
	command.Args[0] = "axi-daemon"
	command.Stdout = logFile
	command.Stderr = logFile
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := command.Start(); err != nil {
		return nil, err
	}
	return command, nil
}
