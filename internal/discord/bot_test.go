package discord

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/config"
	"github.com/DaviRodrigues/opspulse/internal/database"
	"github.com/DaviRodrigues/opspulse/internal/domain"
	"github.com/DaviRodrigues/opspulse/internal/file"
)

func TestBot_Validation(t *testing.T) {
	_, err := New(&config.DiscordConfig{})
	if err == nil {
		t.Errorf("esperava erro ao passar token e channel vazios, mas retornou nil")
	}
}

func TestRunOnDemandChecks_FetchesEnabledAndPersistsLogs(t *testing.T) {
	srvUp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srvUp.Close()

	srvDown := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srvDown.Close()

	ctx := context.Background()
	targetRepo := database.NewMockTargetRepository()
	logRepo := database.NewMockCheckLogRepository()

	_ = targetRepo.Create(ctx, &domain.Target{
		Name:    "Active Up",
		URL:     srvUp.URL,
		Enabled: true,
	})
	_ = targetRepo.Create(ctx, &domain.Target{
		Name:    "Active Down",
		URL:     srvDown.URL,
		Enabled: true,
	})
	_ = targetRepo.Create(ctx, &domain.Target{
		Name:    "Disabled",
		URL:     "http://127.0.0.1:1",
		Enabled: false,
	})

	results := runOnDemandChecks(ctx, targetRepo, logRepo, 2*time.Second)
	if len(results) != 2 {
		t.Fatalf("expected 2 results for enabled targets, got %d", len(results))
	}

	logs, total, err := logRepo.FindAll(ctx, database.CheckLogFilter{}, 10, 0)
	if err != nil {
		t.Fatalf("unexpected FindAll error: %v", err)
	}
	if total != 2 || len(logs) != 2 {
		t.Fatalf("expected 2 persisted check logs, got total=%d len=%d", total, len(logs))
	}

	// Verify dynamic updates from repository are picked up on subsequent runs
	_ = targetRepo.Create(ctx, &domain.Target{
		Name:    "New Dynamic Target",
		URL:     srvUp.URL + "/v2",
		Enabled: true,
	})
	resultsAfter := runOnDemandChecks(ctx, targetRepo, logRepo, 2*time.Second)
	if len(resultsAfter) != 3 {
		t.Fatalf("expected 3 results after adding new enabled target, got %d", len(resultsAfter))
	}
}

func TestCreateStatusSummaryEmbed_EmptyResults(t *testing.T) {
	embeds := createStatusSummaryEmbed(nil)
	if len(embeds) != 1 {
		t.Fatalf("expected 1 informational embed for empty results, got %d", len(embeds))
	}
	if embeds[0].Title == "" || embeds[0].Description == "" {
		t.Errorf("expected non-empty title and description on fallback embed: %+v", embeds[0])
	}
}

func TestBot_Integration(t *testing.T) {
	// Caso queira testar, o ideal é colocar .env na raiz do módulo discord
	_ = file.NewEnvFile()
	token := os.Getenv("DISCORD_TOKEN")
	channelID := os.Getenv("DISCORD_CHANNEL_ID")
	if token == "" {
		t.Skip("Pulando teste de integração real: DISCORD_TOKEN não configurado no ambiente")
	}
	if channelID == "" {
		t.Skip("Pulando teste de integração real: DISCORD_CHANNEL_ID não configurado no ambiente")
	}

	bot, err := New(&config.DiscordConfig{
		Token:     token,
		ChannelID: channelID,
		GuildID:   "",
	})
	if err != nil {
		t.Fatalf("falha ao conectar ao discord: %v", err)
	}
	defer bot.Close()
}
