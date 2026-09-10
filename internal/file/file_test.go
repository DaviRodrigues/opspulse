package file

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestJSONFile_CreateAndLoad_Default(t *testing.T) {
	tmpDir := t.TempDir()
	jsonFile := &JSONFile{
		FileDefault: FileDefault{
			Name: "targets.json",
			Path: tmpDir,
		},
	}

	if jsonFile.FileExists() {
		t.Fatalf("o arquivo não deveria existir inicialmente")
	}

	targets, err := jsonFile.Load()
	if err != nil {
		t.Fatalf("não esperava erro ao carregar/criar targets padrão, recebeu: %v", err)
	}

	if len(targets) != 2 {
		t.Fatalf("esperava 2 targets padrão, recebeu: %d", len(targets))
	}

	if targets[0].Name != "Google" || targets[0].URL != "https://www.google.com" {
		t.Errorf("target 0 inválido: %+v", targets[0])
	}

	if !jsonFile.FileExists() {
		t.Errorf("o arquivo deveria existir fisicamente após Load()")
	}
}

func TestJSONFile_Load_CustomValid(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "custom.json")

	customJSON := `[
		{
			"name": "Meu Servidor",
			"url": "https://meuservidor.com",
			"enabled": true,
			"timeout": 3000000000
		}
	]`

	if err := os.WriteFile(filePath, []byte(customJSON), 0644); err != nil {
		t.Fatalf("falha ao preparar arquivo de teste: %v", err)
	}

	jsonFile := &JSONFile{
		FileDefault: FileDefault{
			Name: "custom.json",
			Path: tmpDir,
		},
	}

	targets, err := jsonFile.Load()
	if err != nil {
		t.Fatalf("não esperava erro ao carregar JSON customizado: %v", err)
	}

	if len(targets) != 1 {
		t.Fatalf("esperava 1 target, recebeu: %d", len(targets))
	}

	if targets[0].Name != "Meu Servidor" || targets[0].URL != "https://meuservidor.com" {
		t.Errorf("dados do target incorretos: %+v", targets[0])
	}
}

func TestJSONFile_Validate_InvalidURL(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "invalid.json")

	invalidJSON := `[
		{
			"name": "URL Sem Protocolo",
			"url": "localhost:8080",
			"enabled": true
		}
	]`

	if err := os.WriteFile(filePath, []byte(invalidJSON), 0644); err != nil {
		t.Fatalf("falha ao preparar arquivo de teste: %v", err)
	}

	jsonFile := &JSONFile{
		FileDefault: FileDefault{
			Name: "invalid.json",
			Path: tmpDir,
		},
	}

	_, err := jsonFile.Load()
	if err == nil {
		t.Errorf("esperava erro de validação para URL sem http/https, mas retornou nil")
	}
}

func TestJSONFile_Validate_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "empty.json")

	if err := os.WriteFile(filePath, []byte("[]"), 0644); err != nil {
		t.Fatalf("falha ao preparar arquivo de teste: %v", err)
	}

	jsonFile := &JSONFile{
		FileDefault: FileDefault{
			Name: "empty.json",
			Path: tmpDir,
		},
	}

	_, err := jsonFile.Load()
	if err == nil {
		t.Errorf("esperava erro ao validar lista vazia, mas retornou nil")
	}
}

func TestEnvFile_Helpers(t *testing.T) {
	envFile := &EnvFile{}

	t.Setenv("TEST_VAR", "meu_valor")
	t.Setenv("TEST_DURATION", "45s")
	t.Setenv("TEST_LIST", "https://a.com, https://b.com")

	val, err := envFile.LoadVariable("TEST_VAR", "fallback")
	if err != nil || val != "meu_valor" {
		t.Errorf("esperava 'meu_valor', recebeu: %s (err: %v)", val, err)
	}

	dur, err := envFile.LoadDurationEnv("TEST_DURATION")
	if err != nil || dur != 45*time.Second {
		t.Errorf("esperava 45s, recebeu: %v (err: %v)", dur, err)
	}

	list, err := envFile.LoadListEnv("TEST_LIST")
	if err != nil || len(list) != 2 {
		t.Errorf("esperava lista com 2 itens, recebeu: %v (err: %v)", list, err)
	}
}
