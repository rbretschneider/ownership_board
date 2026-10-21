package secretbox_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/rbretschneider/ownership_board/internal/secretbox"
)

const canary = "ghu_CANARY_do_not_leak"

func newBox(t *testing.T, master string) *secretbox.Box {
	t.Helper()
	key, err := secretbox.DeriveKey(master, "test/v1")
	if err != nil {
		t.Fatalf("DeriveKey: %v", err)
	}
	box, err := secretbox.New(key)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return box
}

func seal(t *testing.T, box *secretbox.Box, value, ad string) string {
	t.Helper()
	sealed, err := box.Seal(secretbox.NewSecret(value), ad)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	return sealed
}

func TestRoundTripWithMatchingAssociatedData(t *testing.T) {
	box := newBox(t, "master secret")
	sealed := seal(t, box, canary, "github/access")
	if strings.Contains(sealed, canary) {
		t.Fatal("plaintext visible in sealed value")
	}
	opened, err := box.Open(sealed, "github/access")
	if err != nil || opened.Reveal() != canary {
		t.Fatalf("Open = %v, %v", opened.Reveal(), err)
	}
}

func TestSealedValueCannotMoveToAnotherColumn(t *testing.T) {
	box := newBox(t, "master secret")
	sealed := seal(t, box, canary, "github/access")
	if _, err := box.Open(sealed, "github/refresh"); !errors.Is(err, secretbox.ErrCannotOpen) {
		t.Fatalf("opened with other associated data: %v", err)
	}
}

func TestChangedMasterSecretIsReportedAsWrongKey(t *testing.T) {
	sealed := seal(t, newBox(t, "one"), canary, "ad")
	if _, err := newBox(t, "two").Open(sealed, "ad"); !errors.Is(err, secretbox.ErrWrongKey) {
		t.Fatalf("opened under another key: %v", err)
	}
}

func TestSealingTwiceGivesDifferentCiphertexts(t *testing.T) {
	box := newBox(t, "master secret")
	if seal(t, box, "same", "ad") == seal(t, box, "same", "ad") {
		t.Fatal("nonce reuse: identical ciphertexts")
	}
}

func TestTamperedValueFailsToOpen(t *testing.T) {
	box := newBox(t, "master secret")
	sealed := seal(t, box, canary, "ad")
	tampered := sealed[:len(sealed)-2] + "AA"
	if _, err := box.Open(tampered, "ad"); !errors.Is(err, secretbox.ErrCannotOpen) {
		t.Fatalf("tampered value opened: %v", err)
	}
}

func TestSecretNeverPrints(t *testing.T) {
	secret := secretbox.NewSecret(canary)
	var logged bytes.Buffer
	slog.New(slog.NewTextHandler(&logged, nil)).Info("x", "token", secret)
	encoded, _ := json.Marshal(struct{ Token secretbox.Secret }{secret})
	wrapped := struct{ Token secretbox.Secret }{secret}
	outputs := []string{
		fmt.Sprint(secret), fmt.Sprintf("%v %+v %#v %s %d %t %x %q", secret, secret, secret, secret, secret, secret, secret, secret),
		fmt.Sprintf("%v %+v %#v %d", wrapped, wrapped, wrapped, wrapped), string(encoded), logged.String(),
	}
	for _, output := range outputs {
		if strings.Contains(output, canary) {
			t.Fatalf("secret leaked: %s", output)
		}
	}
}
