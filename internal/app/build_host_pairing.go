package app

import (
	"io"
)

const pairingAlphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"

// The alphabet has exactly 32 symbols, so each masked byte is uniform.
func generatePairingCode(entropy io.Reader) (string, error) {
	var data [26]byte
	if _, err := io.ReadFull(entropy, data[:]); err != nil {
		return "", err
	}
	for i, b := range data {
		data[i] = pairingAlphabet[b&31]
	}
	return string(data[:]), nil
}
