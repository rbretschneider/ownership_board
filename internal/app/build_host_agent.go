package app

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"strings"
	"time"

	"github.com/rbretschneider/ownership_board/internal/auth"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

type AuthenticatedBuildHost struct{ ID int64 }
type BuildHostPairInput struct {
	Code      string          `json:"code"`
	PublicKey string          `json:"public_key"`
	Name      string          `json:"name"`
	Facts     json.RawMessage `json:"facts"`
}

// Format and LogValue prevent accidental enrollment credential logging.
func (in BuildHostPairInput) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "[redacted]")
}
func (in BuildHostPairInput) LogValue() slog.Value { return slog.StringValue("[redacted]") }
func (in BuildHostPairInput) MarshalJSON() ([]byte, error) {
	type wire BuildHostPairInput
	safe := wire(in)
	safe.Code = "[redacted]"
	return json.Marshal(safe)
}

func (s *BuildHostService) Now() time.Time { return s.clock.Now() }
func (s *BuildHostService) Pair(ctx context.Context, in BuildHostPairInput) (domain.BuildHost, error) {
	f, e := domain.ParseBuildHostFacts(in.Facts)
	if e != nil {
		return domain.BuildHost{}, e
	}
	key, e := base64.StdEncoding.DecodeString(in.PublicKey)
	if e != nil || len(key) != 32 || base64.StdEncoding.EncodeToString(key) != in.PublicKey {
		return domain.BuildHost{}, domain.Invalidf("invalid public key")
	}
	now := s.clock.Now().UTC().Truncate(time.Millisecond)
	h, e := domain.NewBuildHost(domain.BuildHostDraft{Name: in.Name, OS: f.OS, Arch: f.Arch, PublicKey: in.PublicKey, Facts: in.Facts, AgentVersion: f.AgentVersion, Capabilities: domain.SuggestedBuildHostCapabilities(f)}, now)
	if e != nil {
		return h, e
	}
	digest := sha256.Sum256([]byte(in.Code))
	hash := hex.EncodeToString(digest[:])
	valid := len(in.Code) == 26
	for _, c := range in.Code {
		if !strings.ContainsRune(pairingAlphabet, c) {
			valid = false
		}
	}
	e = s.store.Write(ctx, func(r Repos) error {
		now = s.clock.Now().UTC().Truncate(time.Millisecond)
		h.CreatedAt = now
		c, e := r.BuildHostPairingCodes().GetByHash(ctx, hash)
		if e != nil {
			if domain.KindOf(e) != domain.KindNotFound {
				return e
			}
			subtle.ConstantTimeCompare(digest[:], make([]byte, 32))
			return auth.ErrUnauthorized
		}
		stored, e := hex.DecodeString(c.CodeHash)
		if e != nil || len(stored) != 32 {
			return errors.New("invalid stored pairing digest")
		}
		if subtle.ConstantTimeCompare(digest[:], stored) != 1 || !valid || c.UsedAt != nil || !c.ExpiresAt.After(now) {
			return auth.ErrUnauthorized
		}
		if e = r.BuildHosts().Insert(ctx, &h); e != nil {
			return e
		}
		return r.BuildHostPairingCodes().Consume(ctx, hash, h.ID, now)
	})
	if e != nil {
		return domain.BuildHost{}, e
	}
	if s.notifier != nil {
		notifyCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if e = s.notifier.Notify(notifyCtx, BuildHostPendingNotification()); e != nil {
			s.logger.WarnContext(ctx, "build host notification failed", "host_id", h.ID)
		}
	}
	return h, nil
}
func (s *BuildHostService) AuthenticateHost(ctx context.Context, id int64, method, path, ts, sig string, body []byte, allowPending bool) (AuthenticatedBuildHost, error) {
	var h domain.BuildHost
	e := s.store.Read(ctx, func(r Repos) error { var e error; h, e = r.BuildHosts().Get(ctx, id); return e })
	if e != nil {
		if domain.KindOf(e) == domain.KindNotFound {
			return AuthenticatedBuildHost{}, auth.ErrUnauthorized
		}
		return AuthenticatedBuildHost{}, e
	}
	key, e := base64.StdEncoding.DecodeString(h.PublicKey)
	if e != nil {
		return AuthenticatedBuildHost{}, errors.New("invalid stored host key")
	}
	if h.Status != domain.BuildHostApproved && !(allowPending && method == "GET" && path == "/api/v1/build-host/status" && h.Status == domain.BuildHostPending) || !auth.VerifyHostSignature(ed25519.PublicKey(key), method, path, ts, sig, body, s.clock.Now()) {
		return AuthenticatedBuildHost{}, auth.ErrUnauthorized
	}
	return AuthenticatedBuildHost{ID: id}, nil
}
func (s *BuildHostService) HostStatus(ctx context.Context, who AuthenticatedBuildHost) (domain.BuildHost, error) {
	var h domain.BuildHost
	e := s.store.Read(ctx, func(r Repos) error { var e error; h, e = r.BuildHosts().Get(ctx, who.ID); return e })
	if domain.KindOf(e) == domain.KindNotFound {
		return h, auth.ErrUnauthorized
	}
	if e == nil && h.Status != domain.BuildHostPending && h.Status != domain.BuildHostApproved {
		return h, auth.ErrUnauthorized
	}
	return h, e
}
func toolchains(raw json.RawMessage) map[string]string {
	var obj struct {
		Toolchains map[string]string `json:"toolchains"`
	}
	json.Unmarshal(raw, &obj)
	return obj.Toolchains
}
func (s *BuildHostService) Heartbeat(ctx context.Context, who AuthenticatedBuildHost, report domain.BuildHostReport) (domain.BuildHost, error) {
	if e := report.Validate(); e != nil {
		return domain.BuildHost{}, e
	}
	if report.RunningJobIDs == nil {
		return domain.BuildHost{}, domain.Invalidf("running jobs array required")
	}
	var h domain.BuildHost
	e := s.store.Write(ctx, func(r Repos) error {
		var e error
		h, e = r.BuildHosts().Get(ctx, who.ID)
		if domain.KindOf(e) == domain.KindNotFound {
			return auth.ErrUnauthorized
		}
		if e != nil {
			return e
		}
		if h.Status != domain.BuildHostApproved {
			return auth.ErrUnauthorized
		}
		now := s.clock.Now().UTC().Truncate(time.Millisecond)
		before, after := toolchains(h.Facts), toolchains(report.Facts)
		if !maps.Equal(before, after) {
			if before == nil {
				before = map[string]string{}
			}
			data, _ := json.Marshal(map[string]any{"before": before, "after": after})
			if e = r.BuildHostEvents().Append(ctx, &domain.BuildHostEvent{HostID: h.ID, Type: "toolchains_changed", Data: data, CreatedAt: now}); e != nil {
				return e
			}
		}
		if e = r.BuildHosts().RecordHeartbeat(ctx, h.ID, report, now); e != nil {
			return e
		}
		h, e = r.BuildHosts().Get(ctx, h.ID)
		return e
	})
	return h, e
}
func (s *BuildHostService) Events(ctx context.Context, actor domain.Actor, id, after int64, limit int) ([]domain.BuildHostEvent, error) {
	if e := requireBuildHostHuman(actor); e != nil {
		return nil, e
	}
	if after < 0 || limit < 1 || limit > 200 {
		return nil, domain.Invalidf("invalid pagination")
	}
	var events []domain.BuildHostEvent
	e := s.store.Read(ctx, func(r Repos) error {
		if _, e := r.BuildHosts().Get(ctx, id); e != nil {
			return e
		}
		var e error
		events, e = r.BuildHostEvents().ForHost(ctx, id, after, limit)
		return e
	})
	return events, e
}
