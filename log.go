package main

import (
	"os"
	"path/filepath"
	"sync"
)

const maxLogSize = 1 << 20

type rollingLog struct {
	path string
	file *os.File
	size int64
	mu   sync.Mutex
}

func newRollingLog(home string) (*rollingLog, error) {
	directory := filepath.Join(home, "logs")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	logger := &rollingLog{path: filepath.Join(directory, "axi.log")}
	if info, err := os.Stat(logger.path); err == nil && info.Size() >= maxLogSize {
		if err := logger.rotate(); err != nil {
			return nil, err
		}
	}
	if err := logger.open(); err != nil {
		return nil, err
	}
	return logger, nil
}

func (logger *rollingLog) Write(data []byte) (int, error) {
	originalSize := len(data)
	if len(data) > maxLogSize {
		data = data[len(data)-maxLogSize:]
	}
	logger.mu.Lock()
	defer logger.mu.Unlock()
	if logger.size+int64(len(data)) > maxLogSize {
		if err := logger.file.Close(); err != nil {
			return 0, err
		}
		logger.file = nil
		if err := logger.rotate(); err != nil {
			return 0, err
		}
		if err := logger.open(); err != nil {
			return 0, err
		}
	}
	written, err := logger.file.Write(data)
	logger.size += int64(written)
	if err != nil {
		return written, err
	}
	return originalSize, nil
}

func (logger *rollingLog) Close() error {
	logger.mu.Lock()
	defer logger.mu.Unlock()
	if logger.file == nil {
		return nil
	}
	return logger.file.Close()
}

func (logger *rollingLog) open() error {
	file, err := os.OpenFile(logger.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return err
	}
	logger.file = file
	logger.size = info.Size()
	return nil
}

func (logger *rollingLog) rotate() error {
	backup := logger.path + ".1"
	if err := os.Remove(backup); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(logger.path, backup); err != nil && !os.IsNotExist(err) {
		return err
	}
	logger.size = 0
	return nil
}
