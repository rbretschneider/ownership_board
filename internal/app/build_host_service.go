package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
	"github.com/rbretschneider/ownership_board/internal/secretbox"
)

type BuildHostService struct {
	store  Store
	clock  Clock
	logger *slog.Logger
}

func NewBuildHostService(store Store, clock Clock, logger *slog.Logger) *BuildHostService {
	if logger == nil {
		logger = slog.Default()
	}
	return &BuildHostService{store: store, clock: clock, logger: logger}
}

// PairingCodeIssue holds plaintext only until the API response is written.
type PairingCodeIssue struct {
	Code      secretbox.Secret `json:"-"`
	ExpiresAt time.Time        `json:"expires_at"`
}

func requireBuildHostHuman(actor domain.Actor) error {
	if !actor.IsHuman() || actor.ID <= 0 {
		return domain.Forbiddenf("build host management requires a signed-in person")
	}
	return nil
}

func (s *BuildHostService) List(ctx context.Context, actor domain.Actor) ([]domain.BuildHost, error) {
	if err := requireBuildHostHuman(actor); err != nil {
		return nil, err
	}
	var hosts []domain.BuildHost
	err := s.store.Read(ctx, func(r Repos) error { var err error; hosts, err = r.BuildHosts().List(ctx); return err })
	return hosts, err
}

func (s *BuildHostService) Get(ctx context.Context, actor domain.Actor, id int64) (domain.BuildHost, error) {
	if err := requireBuildHostHuman(actor); err != nil {
		return domain.BuildHost{}, err
	}
	var h domain.BuildHost
	err := s.store.Read(ctx, func(r Repos) error { var err error; h, err = r.BuildHosts().Get(ctx, id); return err })
	return h, err
}

func (s *BuildHostService) CreatePairingCode(ctx context.Context, actor domain.Actor) (PairingCodeIssue, error) {
	if err := requireBuildHostHuman(actor); err != nil {
		return PairingCodeIssue{}, err
	}
	code, err := generatePairingCode(rand.Reader)
	if err != nil {
		return PairingCodeIssue{}, fmt.Errorf("generate pairing code: %w", err)
	}
	now := s.clock.Now().UTC().Truncate(time.Millisecond)
	expires := now.Add(15 * time.Minute)
	record := domain.BuildHostPairingCode{CodeHash: fmt.Sprintf("%x", sha256.Sum256([]byte(code))), CreatedBy: actor.ID, CreatedAt: now, ExpiresAt: expires}
	if err := s.store.Write(ctx, func(r Repos) error { return r.BuildHostPairingCodes().Insert(ctx, record) }); err != nil {
		return PairingCodeIssue{}, err
	}
	s.audit(ctx, "build_host.pairing_code_issued", actor, now, 0, "expires_at", expires)
	return PairingCodeIssue{Code: secretbox.NewSecret(code), ExpiresAt: expires}, nil
}

func (s *BuildHostService) Approve(ctx context.Context, actor domain.Actor, id int64, edit domain.BuildHostEdit) (domain.BuildHost, error) {
	return s.change(ctx, actor, id, "build_host.approved", func(r Repos, h *domain.BuildHost, now time.Time) (bool, error) {
		if err := h.Approve(actor.ID, now, edit); err != nil {
			return false, err
		}
		return true, r.BuildHosts().Approve(ctx, *h)
	})
}

func (s *BuildHostService) Update(ctx context.Context, actor domain.Actor, id int64, edit domain.BuildHostEdit) (domain.BuildHost, error) {
	return s.change(ctx, actor, id, "build_host.updated", func(r Repos, h *domain.BuildHost, _ time.Time) (bool, error) {
		before := *h
		if err := h.ApplyEdit(edit); err != nil {
			return false, err
		}
		if before.Name == h.Name && slices.Equal(before.Capabilities, h.Capabilities) && before.MaxParallel == h.MaxParallel && before.Enabled == h.Enabled {
			return false, nil
		}
		return true, r.BuildHosts().Update(ctx, *h)
	})
}

func (s *BuildHostService) Revoke(ctx context.Context, actor domain.Actor, id int64) (domain.BuildHost, error) {
	return s.change(ctx, actor, id, "build_host.revoked", func(r Repos, h *domain.BuildHost, now time.Time) (bool, error) {
		if h.Status == domain.BuildHostRevoked {
			return false, nil
		}
		h.Revoke(now)
		return true, r.BuildHosts().Revoke(ctx, id, now)
	})
}

func (s *BuildHostService) change(ctx context.Context, actor domain.Actor, id int64, event string, fn func(Repos, *domain.BuildHost, time.Time) (bool, error)) (domain.BuildHost, error) {
	if err := requireBuildHostHuman(actor); err != nil {
		return domain.BuildHost{}, err
	}
	var h domain.BuildHost
	var changed bool
	var now time.Time
	err := s.store.Write(ctx, func(r Repos) error {
		var err error
		h, err = r.BuildHosts().Get(ctx, id)
		if err != nil {
			return err
		}
		now = s.clock.Now().UTC().Truncate(time.Millisecond)
		changed, err = fn(r, &h, now)
		return err
	})
	if err != nil {
		return domain.BuildHost{}, err
	}
	if changed {
		s.audit(ctx, event, actor, now, id, "name", h.Name, "capabilities", h.Capabilities, "max_parallel", h.MaxParallel, "enabled", h.Enabled)
	}
	return h, nil
}

func (s *BuildHostService) audit(ctx context.Context, event string, actor domain.Actor, now time.Time, hostID int64, fields ...any) {
	args := []any{"actor_id", actor.ID, "actor_name", actor.Name, "event_time", now}
	if hostID != 0 {
		args = append(args, "host_id", hostID)
	}
	args = append(args, fields...)
	s.logger.InfoContext(ctx, event, args...)
}
