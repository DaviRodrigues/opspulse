package logger

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/config"
)

func TestSetupLogger(t *testing.T) {
	tmpDir := t.TempDir()

	handler, err := HandlerDefaultText(slog.LevelInfo, tmpDir)
	if err != nil {
		t.Errorf("%v", err)
	}

	_, err = SetupSlog(
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

func TestInitLogger_StructuredMetadata(t *testing.T) {
	tmpDir := t.TempDir()

	appCfg := config.AppConfig{
		Name: "opspulse-test",
		Env:  "staging",
	}
	logCfg := config.LogConfig{
		Level:     slog.LevelDebug,
		Format:    "text",
		OutputDir: tmpDir,
	}

	appLogger, err := InitLogger(appCfg, logCfg)
	if err != nil {
		t.Fatalf("não esperava erro ao inicializar logger: %v", err)
	}

	appLogger.Debug("mensagem de debug com metadados")

	expectedFile := filepath.Join(tmpDir, fmt.Sprintf("opspulse-%s.log", time.Now().Format("2006-01-02")))
	contentBytes, err := os.ReadFile(expectedFile)
	if err != nil {
		t.Fatalf("erro ao ler arquivo de log: %v", err)
	}

	content := string(contentBytes)
	if !strings.Contains(content, "app=opspulse-test") {
		t.Errorf("log não continha atributo app: %s", content)
	}
	if !strings.Contains(content, "env=staging") {
		t.Errorf("log não continha atributo env: %s", content)
	}
	if !strings.Contains(content, "mensagem de debug com metadados") {
		t.Errorf("log não continha mensagem esperada: %s", content)
	}
}

func TestInitLogger_JSONFormat(t *testing.T) {
	tmpDir := t.TempDir()

	appCfg := config.AppConfig{
		Name: "json-service",
		Env:  "production",
	}
	logCfg := config.LogConfig{
		Level:     slog.LevelInfo,
		Format:    "json",
		OutputDir: tmpDir,
	}

	appLogger, err := InitLogger(appCfg, logCfg)
	if err != nil {
		t.Fatalf("erro ao inicializar logger JSON: %v", err)
	}

	appLogger.Info("evento json estruturado", "status_code", 200)

	expectedFile := filepath.Join(tmpDir, fmt.Sprintf("opspulse-%s.log", time.Now().Format("2006-01-02")))
	contentBytes, err := os.ReadFile(expectedFile)
	if err != nil {
		t.Fatalf("erro ao ler arquivo de log: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(contentBytes)), "\n")
	lastLine := lines[len(lines)-1]

	var parsed map[string]any
	if err := json.Unmarshal([]byte(lastLine), &parsed); err != nil {
		t.Fatalf("linha de log não é um JSON válido: %s, erro: %v", lastLine, err)
	}

	if parsed["app"] != "json-service" {
		t.Errorf("esperava app='json-service', recebeu: %v", parsed["app"])
	}
	if parsed["env"] != "production" {
		t.Errorf("esperava env='production', recebeu: %v", parsed["env"])
	}
	if parsed["msg"] != "evento json estruturado" {
		t.Errorf("esperava msg='evento json estruturado', recebeu: %v", parsed["msg"])
	}
}

