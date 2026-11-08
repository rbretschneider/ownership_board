// Package buildbundle stores opaque commit payloads without inspecting their contents.
package buildbundle

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

type Store struct{ dir string }

func New(dir string) (*Store, error) {
	if e := os.MkdirAll(dir, 0700); e != nil {
		return nil, e
	}
	info, e := os.Lstat(dir)
	if e != nil {
		return nil, e
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("bundle root must be a directory")
	}
	s := &Store{dir: dir}
	_, e = s.PayloadSizes()
	return s, e
}
func (s *Store) root() (*os.Root, error) {
	info, e := os.Lstat(s.dir)
	if e != nil {
		return nil, e
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("unsafe bundle root")
	}
	return os.OpenRoot(s.dir)
}
func (s *Store) PayloadSizes() (map[string]int64, error) {
	r, e := s.root()
	if e != nil {
		return nil, e
	}
	defer r.Close()
	out := map[string]int64{}
	e = fs.WalkDir(r.FS(), ".", func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in bundle storage")
		}
		if d.IsDir() {
			if p != "." && !domain.ValidBuildJobID(p) {
				return fmt.Errorf("unexpected bundle directory")
			}
			return nil
		}
		info, e := d.Info()
		if e != nil {
			return e
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unexpected bundle file type")
		}
		base := filepath.Base(p)
		if base != "commit.bundle" && !(len(base) == 72 && base[64:] == ".partial" && domain.ValidBuildJobID(base[:64])) {
			return fmt.Errorf("unexpected bundle file")
		}
		out[p] = info.Size()
		return nil
	})
	return out, e
}
func (s *Store) Stage(token string, m domain.BuildJobSubmission, src io.Reader) error {
	if !domain.ValidBuildJobID(token) {
		return domain.Invalidf("invalid upload token")
	}
	if m.BundleBytes <= 0 || m.BundleBytes > domain.BuildJobBundleLimit {
		return domain.TooLargef("bundle exceeds 100 MiB")
	}
	r, e := s.root()
	if e != nil {
		return e
	}
	defer r.Close()
	f, e := r.OpenFile(token+".partial", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	hash := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(f, hash), io.LimitReader(src, m.BundleBytes))
	if copyErr == nil && n == m.BundleBytes {
		var b [1]byte
		extra, readErr := io.ReadFull(src, b[:])
		if extra > 0 {
			copyErr = domain.Invalidf("bundle exceeds declared size")
			if m.BundleBytes == domain.BuildJobBundleLimit {
				copyErr = domain.TooLargef("bundle exceeds 100 MiB")
			}
		} else if readErr != io.EOF {
			copyErr = readErr
		}
	}
	if copyErr == nil && (n != m.BundleBytes || fmt.Sprintf("%x", hash.Sum(nil)) != m.BundleSHA256) {
		copyErr = domain.Invalidf("bundle size or digest mismatch")
	}
	if copyErr == nil {
		copyErr = f.Sync()
	}
	return errors.Join(copyErr, f.Close())
}
func (s *Store) Promote(token string, m domain.BuildJobSubmission) error {
	if !domain.ValidBuildJobID(token) {
		return domain.Invalidf("invalid upload token")
	}
	r, e := s.root()
	if e != nil {
		return e
	}
	defer r.Close()
	id := m.ID()
	if e = r.Mkdir(id, 0700); e != nil && !errors.Is(e, os.ErrExist) {
		return e
	}
	info, e := r.Lstat(id)
	if e != nil {
		return e
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("unsafe bundle directory")
	}
	target := id + "/commit.bundle"
	if _, e = r.Lstat(target); e == nil {
		if e = s.Verify(id, m.BundleBytes, m.BundleSHA256); e != nil {
			return domain.Conflictf("retained bundle conflicts with upload")
		}
		return nil
	} else if !errors.Is(e, os.ErrNotExist) {
		return e
	}
	// Linking publishes without replacing an existing file, even after a crash.
	if e = r.Link(token+".partial", target); e != nil {
		return e
	}
	d, e := r.Open(id)
	if e != nil {
		return e
	}
	e = errors.Join(d.Sync(), d.Close())
	if e != nil {
		return e
	}
	if e = r.Remove(token + ".partial"); e != nil {
		return e
	}
	d, e = r.Open(".")
	if e != nil {
		return e
	}
	return errors.Join(d.Sync(), d.Close())
}
func (s *Store) Open(id string) (io.ReadCloser, error) {
	if !domain.ValidBuildJobID(id) {
		return nil, domain.Invalidf("invalid job id")
	}
	r, e := s.root()
	if e != nil {
		return nil, e
	}
	defer r.Close()
	for _, p := range []string{id, id + "/commit.bundle"} {
		info, e := r.Lstat(p)
		if e != nil {
			return nil, e
		}
		if info.Mode()&os.ModeSymlink != 0 || p != id && !info.Mode().IsRegular() {
			return nil, fmt.Errorf("unsafe bundle path")
		}
	}
	return r.Open(id + "/commit.bundle")
}
func (s *Store) Verify(id string, size int64, digest string) error {
	f, e := s.Open(id)
	if e != nil {
		return e
	}
	defer f.Close()
	hash := sha256.New()
	n, e := io.Copy(hash, io.LimitReader(f, domain.BuildJobBundleLimit+1))
	if e != nil {
		return e
	}
	if n != size || fmt.Sprintf("%x", hash.Sum(nil)) != digest {
		return fmt.Errorf("bundle integrity check failed")
	}
	return nil
}
