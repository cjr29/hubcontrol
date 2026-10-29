package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
)

const (
	Logging  = true            // Always on for logging. Set level with LogLevel property
	LogLevel = slog.LevelError // Production logging level. Change to Debug for development.
	// LogLevel = slog.LevelInfo // Change to Error for production
	// LogLevel = slog.LevelDebug // Change to Error for production
	logDir  = "/var/log/sensorhub"
	logFile = "/var/log/sensorhub/sensorhub.log"
)

var logger *slog.Logger
var logFh *fileHandler

type fileHandler struct {
	mu   sync.RWMutex
	file *os.File
	h    slog.Handler
}

func configureLogging() {
	var err error
	if err = ensureLogDir(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create log dir: %v\n", err)
		os.Exit(1)
	}
	logFh, err = newFileHandler(logFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open log file: %v\n", err)
		os.Exit(1)
	}
	logger = slog.New(logFh)
	logger.Info(fmt.Sprintf("*** SensorHubControl started *** Logging configured for %s.", LogLevel.String()))
}

func ensureLogDir() error {
	return os.MkdirAll(logDir, 0o755)
}

func newFileHandler(path string) (*fileHandler, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	h := slog.NewTextHandler(f, &slog.HandlerOptions{
		Level:       LogLevel,
		AddSource:   false,
		ReplaceAttr: nil,
	})
	return &fileHandler{file: f, h: h}, nil
}

func (fh *fileHandler) Handle(ctx context.Context, r slog.Record) error {
	formattedTime := r.Time.Format("2006/01/02 15:04:05")
	//add time and message to values
	strs := []string{formattedTime, r.Message, "\n"}
	if r.NumAttrs() != 0 {
		r.Attrs(func(a slog.Attr) bool {
			strs = append(strs, a.Value.String())
			return true
		})
	}
	result := strings.Join(strs, " ")
	b := []byte(result)

	fh.mu.Lock()
	defer fh.mu.Unlock()

	_, err := fh.file.Write(b)

	return err
}

func (fh *fileHandler) Enabled(ctx context.Context, l slog.Level) bool {
	fh.mu.RLock()
	defer fh.mu.RUnlock()
	return fh.h.Enabled(ctx, l)
}

func (fh *fileHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	fh.mu.RLock()
	h := fh.h.WithAttrs(attrs)
	fh.mu.RUnlock()
	return &fileHandler{file: fh.file, h: h}
}

func (fh *fileHandler) WithGroup(name string) slog.Handler {
	fh.mu.RLock()
	h := fh.h.WithGroup(name)
	fh.mu.RUnlock()
	return &fileHandler{file: fh.file, h: h}
}

// Reopen closes the current file and opens a new one at the same path, swapping the handler.
func (fh *fileHandler) Reopen(path string) error {
	newf, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	newh := slog.NewTextHandler(newf, &slog.HandlerOptions{
		Level:       LogLevel,
		AddSource:   false,
		ReplaceAttr: nil,
	})

	fh.mu.Lock()
	oldf := fh.file
	fh.file = newf
	fh.h = newh
	fh.mu.Unlock()

	if oldf != nil {
		_ = oldf.Close()
	}
	return nil
}
