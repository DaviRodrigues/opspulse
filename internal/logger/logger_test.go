package logger

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSetupLogger(t *testing.T) {
	tmpDir := t.TempDir()

	handler, err := HandlerDefaultText(slog.LevelInfo, tmpDir)
	if err != nil {
		t.Errorf("%v", err)
	}

	err = SetupSlog(
		handler,
	)
	if err != nil {
		t.Errorf("%v", err)
	}

	slog.Info("teste de log")

	expectedFile := filepath.Join(tmpDir, fmt.Sprintf("opspulse-%s.log", time.Now().Format("2006-01-02")))
	contentBytes, err := os.ReadFile(expectedFile)
	if err != nil {
		t.Fatalf("erro ao ler arquivo de log: %v", err)
	}

	content := string(contentBytes)
	if !strings.Contains(content, "teste de log") {
		t.Errorf("esperava encontrar 'teste de log' no arquivo, mas o conteúdo foi: %s", content)
	}
}
