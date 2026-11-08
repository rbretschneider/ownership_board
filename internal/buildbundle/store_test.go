package buildbundle

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestUploadLockProcess(t *testing.T) {
	const childEnv = "BUNDLE_LOCK_TEST_ROOT"
	token := strings.Repeat("e", 64)
	if root := os.Getenv(childEnv); root != "" {
		s, err := New(root)
		if err != nil {
			t.Fatal(err)
		}
		lock, err := s.TryLockUpload(token)
		if err != nil || lock == nil {
			t.Fatal(lock, err)
		}
		defer lock.Close()
		fmt.Println("locked")
		_, _ = io.Copy(io.Discard, os.Stdin)
		return
	}
	root := t.TempDir()
	s, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestUploadLockProcess$")
	cmd.Env = append(os.Environ(), childEnv+"="+root)
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	line, err := bufio.NewReader(output).ReadString('\n')
	if err != nil || line != "locked\n" {
		t.Fatal(line, err)
	}
	lock, err := s.TryLockUpload(token)
	if err != nil || lock != nil {
		if lock != nil {
			lock.Close()
		}
		t.Fatal("live process lost lock", err)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	lock, err = s.TryLockUpload(token)
	if err != nil || lock == nil {
		t.Fatal("dead process retained lock", err)
	}
	defer lock.Close()
	sizes, err := s.PayloadSizes()
	if err != nil || len(sizes) != 0 {
		t.Fatal("lock metadata counted as payload", sizes, err)
	}
}

func TestUploadLockRejectsUnsafeFiles(t *testing.T) {
	for _, symlink := range []bool{false, true} {
		t.Run(fmt.Sprint(symlink), func(t *testing.T) {
			root := t.TempDir()
			s, err := New(root)
			if err != nil {
				t.Fatal(err)
			}
			token := strings.Repeat("f", 64)
			path := filepath.Join(root, token+".lock")
			if symlink {
				err = os.Symlink("missing", path)
			} else {
				err = os.WriteFile(path, []byte("unexpected"), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if lock, err := s.TryLockUpload(token); err == nil {
				if lock != nil {
					lock.Close()
				}
				t.Fatal("accepted unsafe lock")
			}
			if _, err := s.PayloadSizes(); err == nil {
				t.Fatal("accepted unsafe lock inventory")
			}
		})
	}
}
