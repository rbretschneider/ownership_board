package httpapi

import (
	"encoding/json"
	"io"
	"math"
	"net/http"
	"os"
	"time"
)

type usageWindow struct {
	Label       string     `json:"label"`
	UsedPercent *float64   `json:"used_percent"`
	ResetsAt    *time.Time `json:"resets_at"`
}
type usageAccount struct {
	Provider   string        `json:"provider"`
	ObservedAt *time.Time    `json:"observed_at"`
	Windows    []usageWindow `json:"windows"`
}
type usageSnapshot struct {
	Accounts []usageAccount `json:"accounts"`
}

// Only reads a small sanitized cache. No provider request or model is started.
func (s *server) accountUsage(w http.ResponseWriter, r *http.Request, _ caller) error {
	snapshot := readUsageSnapshot(s.deps.UsageFile)
	w.Header().Set("Cache-Control", "private, max-age=30")
	writeJSON(w, http.StatusOK, snapshot)
	return nil
}
func readUsageSnapshot(path string) usageSnapshot {
	empty := usageSnapshot{Accounts: []usageAccount{}}
	if path == "" {
		return empty
	}
	file, err := os.Open(path)
	if err != nil {
		return empty
	}
	defer file.Close()
	var snapshot usageSnapshot
	decoder := json.NewDecoder(io.LimitReader(file, 65537))
	if err := decoder.Decode(&snapshot); err != nil {
		return empty
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return empty
	}
	clean := empty
	seen := map[string]bool{}
	for _, account := range snapshot.Accounts {
		if (account.Provider != "codex" && account.Provider != "claude") || seen[account.Provider] {
			continue
		}
		seen[account.Provider] = true
		windows := []usageWindow{}
		for _, window := range account.Windows {
			if len(windows) == 4 {
				break
			}
			if len(window.Label) > 24 || window.UsedPercent == nil || math.IsNaN(*window.UsedPercent) || *window.UsedPercent < 0 || *window.UsedPercent > 100 {
				continue
			}
			windows = append(windows, window)
		}
		account.Windows = windows
		clean.Accounts = append(clean.Accounts, account)
	}
	return clean
}
