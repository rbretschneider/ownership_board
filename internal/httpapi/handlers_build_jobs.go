package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

func (s *server) buildJobRoutes(rt *router) {
	rt.agentOnly("POST /api/v1/build-jobs", s.submitBuildJob)
	rt.agentOnly("GET /api/v1/build-jobs/{id}", s.getBuildJob)
	rt.human("POST /api/v1/build-jobs/{id}/cancel", s.cancelBuildJob)
	rt.signedHostLimit("POST /api/v1/build-host/claim", false, 4096, s.claimBuildJob)
	rt.signedHostLimit("GET /api/v1/build-host/jobs/{id}/bundle", false, 0, s.getBuildBundle)
	rt.signedHostLimit("POST /api/v1/build-host/jobs/{id}/progress", false, 96*1024, s.progressBuildJob)
	rt.signedHostLimit("POST /api/v1/build-host/jobs/{id}/result", false, domain.BuildJobResultLimit+4096, s.completeBuildJob)
}

type uploadCounter struct {
	r     io.Reader
	n     int64
	limit int64
}

func (c *uploadCounter) Read(p []byte) (int, error) {
	if c.n >= c.limit {
		var b [1]byte
		n, e := c.r.Read(b[:])
		if n == 0 {
			return 0, e
		}
		return 0, &http.MaxBytesError{Limit: c.limit}
	}
	if int64(len(p)) > c.limit-c.n {
		p = p[:c.limit-c.n]
	}
	n, e := c.r.Read(p)
	c.n += int64(n)
	return n, e
}
func uploadError(e error) error {
	var large *http.MaxBytesError
	if errors.As(e, &large) {
		return domain.TooLargef("request body exceeds the size limit")
	}
	if e != nil && domain.KindOf(e) == 0 {
		return domain.Invalidf("invalid multipart upload")
	}
	return e
}
func (s *server) submitBuildJob(w http.ResponseWriter, r *http.Request, _ caller) error {
	counter := &uploadCounter{r: http.MaxBytesReader(w, r.Body, domain.BuildJobBundleLimit+64*1024), limit: 64 * 1024}
	r.Body = io.NopCloser(counter)
	mr, e := r.MultipartReader()
	if e != nil {
		return domain.Invalidf("multipart upload required")
	}
	part, e := mr.NextPart()
	if e != nil {
		return uploadError(e)
	}
	if part.FormName() != "metadata" || part.FileName() != "" {
		return domain.Invalidf("metadata must be the first part")
	}
	raw, e := io.ReadAll(io.LimitReader(part, 16*1024+1))
	if e != nil {
		return uploadError(e)
	}
	if len(raw) > 16*1024 {
		return domain.TooLargef("metadata exceeds 16 KiB")
	}
	var m domain.BuildJobSubmission
	if e = domain.DecodeStrictObject(raw, &m); e != nil {
		return e
	}
	if e = m.Validate(); e != nil {
		return e
	}
	counter.limit = m.BundleBytes + 64*1024
	part, e = mr.NextPart()
	if e != nil {
		return uploadError(e)
	}
	if part.FormName() != "bundle" {
		return domain.Invalidf("one bundle part is required")
	}
	j, created, e := s.deps.BuildJobs.Submit(r.Context(), m, part, func() error {
		next, err := mr.NextPart()
		if next != nil {
			return domain.Invalidf("extra multipart part")
		}
		if err != io.EOF {
			return uploadError(err)
		}
		if _, err = io.Copy(io.Discard, counter); err != nil {
			return uploadError(err)
		}
		if counter.n > m.BundleBytes+64*1024 {
			return domain.TooLargef("multipart overhead exceeds 64 KiB")
		}
		return nil
	})
	if e != nil {
		var large *http.MaxBytesError
		if errors.As(e, &large) {
			return domain.TooLargef("request body exceeds the size limit")
		}
		if errors.Is(e, io.ErrUnexpectedEOF) {
			return domain.Invalidf("incomplete multipart upload")
		}
		return e
	}
	status := 200
	if created {
		status = 201
	} else if _, e = io.Copy(io.Discard, counter); e != nil {
		// A duplicate returns the stored job without reading the bundle. Drain
		// it so the client gets this reply: closing on an unread multi-MB body
		// resets a client that sends everything before reading (makercad #159).
		return uploadError(e)
	}
	writeJSON(w, status, j)
	return nil
}
func (s *server) getBuildJob(w http.ResponseWriter, r *http.Request, _ caller) error {
	j, e := s.deps.BuildJobs.Get(r.Context(), r.PathValue("id"))
	if e != nil {
		return e
	}
	writeJSON(w, 200, j)
	return nil
}
func (s *server) cancelBuildJob(w http.ResponseWriter, r *http.Request, who caller) error {
	j, e := s.deps.BuildJobs.Cancel(r.Context(), who.user.IsAdmin, r.PathValue("id"))
	if e != nil {
		return e
	}
	writeJSON(w, 200, j)
	return nil
}
func decodeJobBody(r *http.Request, out any, required ...string) error {
	raw, e := io.ReadAll(r.Body)
	if e != nil {
		return e
	}
	if e = domain.DecodeStrictObject(raw, out); e != nil {
		return e
	}
	var obj map[string]json.RawMessage
	json.Unmarshal(raw, &obj)
	for _, key := range required {
		if _, ok := obj[key]; !ok {
			return domain.Invalidf("missing job field")
		}
	}
	return nil
}
func (s *server) claimBuildJob(w http.ResponseWriter, r *http.Request, who caller) error {
	if e := decodeJobBody(r, &struct{}{}); e != nil {
		return e
	}
	j, e := s.deps.BuildJobs.Claim(r.Context(), who.host)
	if e != nil {
		return e
	}
	if j == nil {
		writeNoContent(w)
	} else {
		writeJSON(w, 200, map[string]any{"job": j})
	}
	return nil
}
func (s *server) getBuildBundle(w http.ResponseWriter, r *http.Request, who caller) error {
	f, j, e := s.deps.BuildJobs.OpenBundle(r.Context(), who.host, r.PathValue("id"))
	if e != nil {
		return e
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(j.BundleBytes, 10))
	w.Header().Set("Cache-Control", "no-store")
	_, e = io.Copy(w, f)
	return e
}
func (s *server) progressBuildJob(w http.ResponseWriter, r *http.Request, who caller) error {
	var p domain.BuildJobProgress
	if e := decodeJobBody(r, &p, "attempt", "phase", "detail"); e != nil {
		return e
	}
	reply, e := s.deps.BuildJobs.Progress(r.Context(), who.host, r.PathValue("id"), p)
	if e != nil {
		return e
	}
	writeJSON(w, 200, reply)
	return nil
}
func (s *server) completeBuildJob(w http.ResponseWriter, r *http.Request, who caller) error {
	var c domain.BuildJobCompletion
	if e := decodeJobBody(r, &c, "attempt", "status", "result"); e != nil {
		return e
	}
	j, e := s.deps.BuildJobs.Complete(r.Context(), who.host, r.PathValue("id"), c)
	if e != nil {
		return e
	}
	writeJSON(w, 200, j)
	return nil
}
