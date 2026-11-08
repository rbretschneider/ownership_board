package app

import (
	"errors"
	"strings"
	"testing"
)

type brokenEntropy struct{}

func (brokenEntropy) Read([]byte) (int, error) { return 0, errors.New("entropy unavailable") }

func TestPairingEntropyFailure(t *testing.T) {
	code, err := generatePairingCode(brokenEntropy{})
	if err == nil || code != "" {
		t.Fatal("entropy failure ignored")
	}
}

func TestPairingUniformMapping(t *testing.T) {
	code, err := generatePairingCode(strings.NewReader(strings.Repeat("\xff", 26)))
	if err != nil || code != strings.Repeat("Z", 26) {
		t.Fatal(code, err)
	}
}
