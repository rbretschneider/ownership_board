package install

import (
	"bytes"
	"encoding/base64"
	assets "github.com/rbretschneider/ownership_board/agent/build_host"
	"testing"
)

func TestRenderedInstallersEmbedCanonicalPayload(t *testing.T) {
	for _, kind := range []string{"sh", "ps1"} {
		raw, err := Render(kind)
		if err != nil {
			t.Fatal(err)
		}
		again, err := Render(kind)
		if err != nil || !bytes.Equal(raw, again) {
			t.Fatal("rendering is not deterministic")
		}
		for _, payload := range [][]byte{assets.Agent, assets.Requirements} {
			encoded := base64.StdEncoding.EncodeToString(payload)
			if !bytes.Contains(raw, []byte(encoded)) {
				t.Fatal("canonical payload missing")
			}
		}
		if bytes.Contains(raw, []byte("@@")) {
			t.Fatal("unexpanded template")
		}
	}
	if _, err := Render("other"); err == nil {
		t.Fatal("unexpected installer")
	}
}
