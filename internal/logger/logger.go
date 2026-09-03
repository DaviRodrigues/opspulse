package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

func makePathLog(logDir string) (io.Writer, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, err
	}

	fileName := fmt.Sprintf("opspulse-%s.log", time.Now().Format("2006-01-02"))
	fullPath := filepath.Join(logDir, fileName)

	file, err := os.OpenFile(fullPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	return io.MultiWriter(os.Stdout, file), nil
}

func SetupSlog(handler slog.Handler) error {
	logger := slog.New(handler)
	slog.SetDefault(logger)
	return nil
}

func HandlerDefaultJSON(level slog.Level, logDir string) (slog.Handler, error) {
	multiWriter, err := makePathLog(logDir)
	if err != nil {
		return nil, err
	}

	return slog.NewJSONHandler(multiWriter, &slog.HandlerOptions{Level: level}), nil
}

func HandlerDefaultText(level slog.Level, logDir string) (slog.Handler, error) {
	multiWriter, err := makePathLog(logDir)
	if err != nil {
		return nil, err
	}

	return slog.NewTextHandler(multiWriter, &slog.HandlerOptions{Level: level}), nil
}