package httpapi_test

import (
	"bytes"
	"github.com/rbretschneider/ownership_board/internal/install"
	"io"
	"net/http"
	"testing"
)

func TestAnonymousInstallers(t *testing.T) {
	browser, _ := signedIn(t)
	for _, kind := range []string{"sh", "ps1"} {
		expected, err := install.Render(kind)
		if err != nil {
			t.Fatal(err)
		}
		for _, method := range []string{"GET", "HEAD"} {
			req, _ := http.NewRequest(method, browser.base+"/install/build-host."+kind, nil)
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := io.ReadAll(res.Body)
			res.Body.Close()
			if res.StatusCode != 200 || res.Header.Get("Content-Type") != "text/plain; charset=utf-8" || res.Header.Get("Cache-Control") != "no-store" || res.Header.Get("X-Content-Type-Options") != "nosniff" {
				t.Fatal(res.StatusCode, res.Header)
			}
			if method == "GET" && !bytes.Equal(raw, expected) {
				t.Fatal("download differs")
			}
			if method == "HEAD" && len(raw) != 0 {
				t.Fatal("HEAD has body")
			}
		}
	}
}
