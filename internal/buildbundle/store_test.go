package buildbundle

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

func TestImmutableBundles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "payloads")
	s, e := New(root)
	if e != nil {
		t.Fatal(e)
	}
	m := domain.BuildJobSubmission{Repo: "example/tools", SHA: strings.Repeat("a", 40), Preset: "script", Target: domain.BuildJobTarget{OS: "linux", RequiredCapabilities: []string{}}, BundleBytes: 3, BundleSHA256: fmt.Sprintf("%x", sha256.Sum256([]byte("abc")))}
	token := strings.Repeat("b", 64)
	if e = s.Stage(token, m, bytes.NewBufferString("abc")); e != nil {
		t.Fatal(e)
	}
	if e = s.Promote(token, m); e != nil {
		t.Fatal(e)
	}
	if e = s.Verify(m.ID(), 3, m.BundleSHA256); e != nil {
		t.Fatal(e)
	}
	sizes, e := s.PayloadSizes()
	if e != nil || sizes[m.ID()+"/commit.bundle"] != 3 {
		t.Fatal(sizes, e)
	}
	if e = s.Stage(strings.Repeat("c", 64), m, bytes.NewBufferString("abcd")); e == nil {
		t.Fatal("accepted excess bytes")
	}
	if e = s.Stage(strings.Repeat("d", 64), m, bytes.NewBufferString("abd")); e == nil {
		t.Fatal("accepted incorrect digest")
	}
	if e = s.Verify(m.ID(), 3, m.BundleSHA256); e != nil {
		t.Fatal("rejection changed bundle", e)
	}
	if e = os.Symlink("commit.bundle", filepath.Join(root, m.ID(), "alias")); e != nil {
		t.Fatal(e)
	}
	if _, e = s.PayloadSizes(); e == nil {
		t.Fatal("accepted symlink")
	}
}
func TestStorageRejectsSymlinkRoot(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(root, "link")
	if e := os.Symlink(root, link); e != nil {
		t.Fatal(e)
	}
	if _, e := New(link); e == nil {
		t.Fatal("accepted root symlink")
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }
func TestBundleSizeBoundary(t *testing.T) {
	root := filepath.Join(t.TempDir(), "payloads")
	s, e := New(root)
	if e != nil {
		t.Fatal(e)
	}
	hash := sha256.New()
	io.Copy(hash, io.LimitReader(zeroReader{}, domain.BuildJobBundleLimit))
	m := domain.BuildJobSubmission{Repo: "example/tools", SHA: strings.Repeat("a", 40), Preset: "script", Target: domain.BuildJobTarget{OS: "linux", RequiredCapabilities: []string{}}, BundleBytes: domain.BuildJobBundleLimit, BundleSHA256: fmt.Sprintf("%x", hash.Sum(nil))}
	if e = s.Stage(strings.Repeat("a", 64), m, io.LimitReader(zeroReader{}, domain.BuildJobBundleLimit)); e != nil {
		t.Fatal(e)
	}
	if e = s.Stage(strings.Repeat("b", 64), m, io.LimitReader(zeroReader{}, domain.BuildJobBundleLimit+1)); domain.KindOf(e) != domain.KindTooLarge {
		t.Fatal("missing 413 classification", e)
	}
}

func TestPromoteRejectsSymlink(t *testing.T) {
	root := filepath.Join(t.TempDir(), "payloads")
	s, e := New(root)
	if e != nil {
		t.Fatal(e)
	}
	token := strings.Repeat("c", 64)
	if e = os.WriteFile(filepath.Join(root, strings.Repeat("d", 64)+".partial"), []byte("abc"), 0600); e != nil {
		t.Fatal(e)
	}
	if e = os.Symlink(strings.Repeat("d", 64)+".partial", filepath.Join(root, token+".partial")); e != nil {
		t.Fatal(e)
	}
	m := domain.BuildJobSubmission{Repo: "example/tools", SHA: strings.Repeat("a", 40), Preset: "script", Target: domain.BuildJobTarget{OS: "linux"}, BundleBytes: 3, BundleSHA256: fmt.Sprintf("%x", sha256.Sum256([]byte("abc")))}
	if e = s.Promote(token, m); e == nil {
		t.Fatal("promoted symlink")
	}
}
