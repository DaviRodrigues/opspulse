package checker

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/contextG"
	"github.com/DaviRodrigues/opspulse/internal/file"
)

func TestCheckURL(t *testing.T) {
	serverSuccess := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
	defer serverSuccess.Close()

	serverError := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			}))
	defer serverError.Close()

	tests := []struct {
		name         string
		url          string
		expectedUp   bool
		expectedCode int
	}{
		{
			name:         "Serviço Saudável (200 OK)",
			url:          serverSuccess.URL,
			expectedUp:   true,
			expectedCode: http.StatusOK,
		},
		{
			name:         "Serviço com Falha (500 Error)",
			url:          serverError.URL,
			expectedUp:   false,
			expectedCode: http.StatusInternalServerError,
		},
		{
			name:         "URL Inexistente/Inválida",
			url:          "http://127.0.0.1:99999",
			expectedUp:   false,
			expectedCode: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checkURL(
				contextG.CreateContext(),
				file.Target{
					Name:    tt.name,
					URL:     tt.url,
					Enabled: true,
					Timeout: 2 * time.Second,
				})

			if result.IsUp != tt.expectedUp {
				t.Errorf("esperava-se IsUp=%v, mas recebeu %v",
					tt.expectedUp, result.IsUp)
			}

			if result.StatusCode != tt.expectedCode {
				t.Errorf("esperava StatusCode=%d, mas recebeu %d",
					tt.expectedCode, result.StatusCode)
			}
		})
	}
}

func TestCheckAll(t *testing.T) {
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server1.Close()
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server2.Close()
	server3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server3.Close()

	timeout := 5 * time.Second
	enabled := true
	targets := []file.Target{
		{URL: server1.URL, Enabled: enabled, Timeout: timeout},
		{URL: server2.URL, Enabled: enabled, Timeout: timeout},
		{URL: server3.URL, Enabled: enabled, Timeout: timeout},
	}

	results := CheckAll(contextG.CreateContext(), targets)
	if len(results) != len(targets) {
		t.Fatalf("esperava %d resultados, mas recebeu %d", len(targets), len(results))
	}

	resultsMap := make(map[string]CheckResult)
	for _, res := range results {
		resultsMap[res.URL] = res
	}

	if res, ok := resultsMap[server1.URL]; !ok || !res.IsUp || res.StatusCode != http.StatusOK {
		t.Errorf("server1 deveria estar UP com 200, recebeu: %+v", res)
	}
	if res, ok := resultsMap[server2.URL]; !ok || res.IsUp || res.StatusCode != http.StatusNotFound {
		t.Errorf("server2 deveria estar DOWN com 404, recebeu: %+v", res)
	}
	if res, ok := resultsMap[server3.URL]; !ok || res.IsUp || res.StatusCode != http.StatusInternalServerError {
		t.Errorf("server3 deveria estar DOWN com 500, recebeu: %+v", res)
	}
}
