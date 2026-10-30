package app

import (
	"context"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// Settings returns the board-wide settings.
func (s *BoardService) Settings(ctx context.Context) (domain.BoardSettings, error) {
	var settings domain.BoardSettings
	err := s.store.Read(ctx, func(repos Repos) error {
		var err error
		settings, err = repos.Settings().Get(ctx)
		return err
	})
	return settings, err
}

// UpdateSettings changes board-wide settings. Only people change them: they
// decide how much unsupervised work runs at once.
func (s *BoardService) UpdateSettings(ctx context.Context, actor domain.Actor, edit domain.SettingsEdit) (domain.BoardSettings, error) {
	if !actor.IsHuman() {
		return domain.BoardSettings{}, domain.Forbiddenf("only a person can change board settings")
	}
	var settings domain.BoardSettings
	err := s.store.Write(ctx, func(repos Repos) error {
		current, err := repos.Settings().Get(ctx)
		if err != nil {
			return err
		}
		if settings, err = current.Apply(edit); err != nil {
			return err
		}
		return repos.Settings().Save(ctx, settings, actor, s.clock.Now())
	})
	if err != nil {
		return domain.BoardSettings{}, err
	}
	return settings, nil
}
