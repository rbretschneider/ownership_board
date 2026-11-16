package config

import (
	"strings"
	"testing"
)

func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestBuildBundleLimitDefaultsTo20GiB(t *testing.T) {
	cfg, err := Load(env(map[string]string{"AUTH_TOKEN_SECRET": strings.Repeat("a", 32)}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BuildBundleLimit != 20<<30 {
		t.Fatalf("BuildBundleLimit = %d, want %d", cfg.BuildBundleLimit, int64(20)<<30)
	}
}

func TestBuildBundleLimitReadsGiB(t *testing.T) {
	cfg, err := Load(env(map[string]string{"AUTH_TOKEN_SECRET": strings.Repeat("a", 32), "BUILD_BUNDLE_STORAGE_GIB": "50"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BuildBundleLimit != 50<<30 {
		t.Fatalf("BuildBundleLimit = %d, want %d", cfg.BuildBundleLimit, int64(50)<<30)
	}
}

func TestBuildBundleLimitRejectsZero(t *testing.T) {
	_, err := Load(env(map[string]string{"AUTH_TOKEN_SECRET": strings.Repeat("a", 32), "BUILD_BUNDLE_STORAGE_GIB": "0"}))
	if err == nil || !strings.Contains(err.Error(), "BUILD_BUNDLE_STORAGE_GIB") {
		t.Fatalf("err = %v, want a BUILD_BUNDLE_STORAGE_GIB problem", err)
	}
}
