package prom

import (
	"fmt"
	"net/http"
	"time"
)

var client = &http.Client{Timeout: 5 * time.Second}

func Reload(baseURL string) error {
	resp, err := client.Post(baseURL+"/-/reload", "", nil)
	if err != nil {
		return fmt.Errorf("reloading prometheus: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("prometheus reload returned status %s", resp.Status)
	}
	return nil
}
