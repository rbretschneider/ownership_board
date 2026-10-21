package reposource_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rbretschneider/ownership_board/internal/reposource"
	"github.com/rbretschneider/ownership_board/internal/secretbox"
)

func TestGitHubPagesUntilAShortPage(t *testing.T) {
	var sawAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		if r.URL.Query().Get("page") == "1" {
			fmt.Fprint(w, "[")
			for i := 0; i < 100; i++ {
				if i > 0 {
					fmt.Fprint(w, ",")
				}
				fmt.Fprintf(w, `{"full_name":"acme/r%d","html_url":"https://github.com/acme/r%d","private":true}`, i, i)
			}
			fmt.Fprint(w, "]")
			return
		}
		fmt.Fprint(w, `[{"full_name":"acme/last","description":null,"archived":true,"default_branch":"main"}]`)
	}))
	defer server.Close()
	listings, err := reposource.NewGitHub(server.Client(), server.URL, secretbox.NewSecret("gh-token")).List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listings) != 101 || !listings[100].IsArchived || listings[100].DefaultBranch != "main" || !listings[0].IsPrivate {
		t.Fatalf("got %d listings, last %+v", len(listings), listings[len(listings)-1])
	}
	if sawAuth != "Bearer gh-token" {
		t.Fatalf("authorization header %q", sawAuth)
	}
}

func TestForgejoPagesUntilEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "token fj-token" || !strings.HasPrefix(r.URL.Path, "/api/v1/user/repos") {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		switch r.URL.Query().Get("page") {
		case "1":
			fmt.Fprint(w, `[{"full_name":"me/a"},{"full_name":"me/b"}]`)
		case "2":
			fmt.Fprint(w, `[{"full_name":"me/c"}]`)
		default:
			fmt.Fprint(w, `[]`)
		}
	}))
	defer server.Close()
	listings, err := reposource.NewForgejo(server.Client(), server.URL, secretbox.NewSecret("fj-token")).List(context.Background())
	if err != nil || len(listings) != 3 {
		t.Fatalf("got %d listings, err %v", len(listings), err)
	}
}

func TestRejectedTokenExplainsItself(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"message":"Bad credentials"}`, http.StatusUnauthorized)
	}))
	defer server.Close()
	_, err := reposource.NewGitHub(server.Client(), server.URL, secretbox.NewSecret("bad")).List(context.Background())
	if err == nil || !strings.Contains(err.Error(), "token was rejected") {
		t.Fatalf("error = %v", err)
	}
}

func TestSourcesReportConfiguration(t *testing.T) {
	client := http.DefaultClient
	if reposource.NewGitHub(client, "", secretbox.Secret{}).IsConfigured() || !reposource.NewGitHub(client, "", secretbox.NewSecret("x")).IsConfigured() {
		t.Fatal("GitHub configuration is wrong")
	}
	if reposource.NewForgejo(client, "https://git.example.com", secretbox.Secret{}).IsConfigured() {
		t.Fatal("Forgejo without a token reported configured")
	}
	if got := reposource.NewForgejo(client, "https://git.example.com", secretbox.NewSecret("x")).Label(); got != "Forgejo (git.example.com)" {
		t.Fatalf("label %q", got)
	}
}
