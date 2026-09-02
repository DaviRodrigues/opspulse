package checker

import (
	"net/http"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/models"
)

func CheckURL(url string, timeout time.Duration) models.CheckResult {
	start := time.Now()
	client := &http.Client{
		Timeout: timeout,
	}

	resp, err := client.Get(url)
	if err != nil {
		return models.CheckResult{
			IsUp: false,
			Error: err,
			Latency: time.Since(start),
			URL: url, 
			StatusCode: 0,
		}
	}

	defer resp.Body.Close()
	return models.CheckResult{
		URL: url, 
		IsUp: resp.StatusCode >= 200 && resp.StatusCode < 400,
		StatusCode: resp.StatusCode,
		Latency: time.Since(start),
		Error: nil, 
	}
}