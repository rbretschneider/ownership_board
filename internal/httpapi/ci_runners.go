package httpapi

import (
	"encoding/json"
	"io"
	"os"
	"regexp"
	"time"
)

// The repositories' GitHub Actions runners, sampled by the merge sweep's CI
// health pass and written as a small sanitized file (like pr_status.json).
// The board only reads it, for the Team view.

type ciRunner struct {
	Repo   string   `json:"repo"`
	Name   string   `json:"name"`
	Status string   `json:"status"`
	Busy   bool     `json:"busy"`
	Labels []string `json:"labels"`
}

type ciRunnersSnapshot struct {
	ObservedAt *time.Time `json:"observed_at"`
	Runners    []ciRunner `json:"runners"`
}

// The file's own shape: epoch seconds for the sample time.
type ciRunnersFile struct {
	At      float64 `json:"at"`
	Runners []struct {
		Repo   string   `json:"repo"`
		Name   string   `json:"name"`
		Status string   `json:"status"`
		Busy   bool     `json:"busy"`
		Labels []string `json:"labels"`
	} `json:"runners"`
}

const (
	maxCIRunners      = 50
	maxCIRunnerLabels = 12
)

var (
	ciRunnerNamePattern  = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)
	ciRunnerLabelPattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,40}$`)
	ciRunnerStatuses     = map[string]bool{"online": true, "offline": true}
)

func readCIRunners(path string) ciRunnersSnapshot {
	empty := ciRunnersSnapshot{Runners: []ciRunner{}}
	if path == "" {
		return empty
	}
	file, err := os.Open(path)
	if err != nil {
		return empty
	}
	defer file.Close()
	var raw ciRunnersFile
	decoder := json.NewDecoder(io.LimitReader(file, 256<<10+1))
	if err := decoder.Decode(&raw); err != nil {
		return empty
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return empty
	}
	clean := empty
	clean.ObservedAt = epoch(raw.At)
	if clean.ObservedAt == nil {
		return empty
	}
	seen := map[string]bool{}
	for _, r := range raw.Runners {
		if len(clean.Runners) == maxCIRunners {
			break
		}
		key := r.Repo + "/" + r.Name
		if !runnerRepoPattern.MatchString(r.Repo) || !ciRunnerNamePattern.MatchString(r.Name) || !ciRunnerStatuses[r.Status] || seen[key] {
			continue
		}
		seen[key] = true
		labels := []string{}
		for _, label := range r.Labels {
			if len(labels) == maxCIRunnerLabels {
				break
			}
			if ciRunnerLabelPattern.MatchString(label) {
				labels = append(labels, label)
			}
		}
		clean.Runners = append(clean.Runners, ciRunner{Repo: r.Repo, Name: r.Name, Status: r.Status, Busy: r.Busy && r.Status == "online", Labels: labels})
	}
	return clean
}
