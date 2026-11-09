package app

import (
	"cmp"
	"context"
	"log/slog"
	"slices"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

func windowsHosts(hosts []domain.BuildHost) []domain.BuildHost {
	out := []domain.BuildHost{}
	for _, h := range hosts {
		if h.Status == domain.BuildHostApproved && h.OS == "windows" {
			out = append(out, h)
		}
	}
	slices.SortFunc(out, func(a, b domain.BuildHost) int { return cmp.Compare(a.ID, b.ID) })
	return out
}
func projectWindowsSettings(settings domain.BoardSettings, hosts []domain.BuildHost) domain.BoardSettings {
	settings.WindowsRunnerEnabled = false
	settings.WindowsParallelBuilds = 0
	settings.WindowsParallelBuildLimit = 0
	approved := windowsHosts(hosts)
	if len(approved) == 0 {
		// No approved Windows host yet: the pre-build-host values stay in
		// force (read-only) so existing Windows verification keeps running.
		if settings.LegacyWindowsRunnerEnabled && settings.LegacyWindowsParallelBuilds > 0 {
			settings.WindowsRunnerEnabled = true
			settings.WindowsParallelBuilds = settings.LegacyWindowsParallelBuilds
		}
		return settings
	}
	for _, h := range approved {
		settings.WindowsParallelBuildLimit += 8
		if h.Enabled {
			settings.WindowsRunnerEnabled = true
			settings.WindowsParallelBuilds += h.MaxParallel
		}
	}
	return settings
}
func loadSettings(ctx context.Context, repos Repos) (domain.BoardSettings, error) {
	settings, err := repos.Settings().Get(ctx)
	if err != nil {
		return settings, err
	}
	hosts, err := repos.BuildHosts().List(ctx)
	if err != nil {
		return domain.BoardSettings{}, err
	}
	return projectWindowsSettings(settings, hosts), nil
}

// Settings returns ordinary configuration with legacy Windows values projected from hosts.
func (s *BoardService) Settings(ctx context.Context) (domain.BoardSettings, error) {
	var settings domain.BoardSettings
	err := s.store.Read(ctx, func(r Repos) error { var err error; settings, err = loadSettings(ctx, r); return err })
	return settings, err
}

// UpdateSettings atomically applies ordinary settings and compatibility host edits.
func (s *BoardService) UpdateSettings(ctx context.Context, actor domain.Actor, edit domain.SettingsEdit) (domain.BoardSettings, error) {
	if !actor.IsHuman() {
		return domain.BoardSettings{}, domain.Forbiddenf("only a person can change board settings")
	}
	var settings domain.BoardSettings
	changed := []domain.BuildHost{}
	err := s.store.Write(ctx, func(r Repos) error {
		current, err := r.Settings().Get(ctx)
		if err != nil {
			return err
		}
		ordinary := edit
		ordinary.WindowsRunnerEnabled = nil
		ordinary.WindowsParallelBuilds = nil
		if settings, err = current.Apply(ordinary); err != nil {
			return err
		}
		hosts, err := r.BuildHosts().List(ctx)
		if err != nil {
			return err
		}
		hosts = windowsHosts(hosts)
		n := len(hosts)
		if n == 0 && edit.WindowsParallelBuilds != nil {
			return domain.Conflictf("register and approve a Windows build host before configuring Windows builds")
		}
		if n == 0 && edit.WindowsRunnerEnabled != nil {
			// Until a Windows host is approved, the pre-build-host switch
			// still turns Windows builds on and off, at its stored capacity.
			if *edit.WindowsRunnerEnabled && settings.LegacyWindowsParallelBuilds == 0 {
				return domain.Conflictf("register and approve a Windows build host before enabling Windows builds")
			}
			settings.LegacyWindowsRunnerEnabled = *edit.WindowsRunnerEnabled
		}
		if edit.WindowsParallelBuilds != nil && (*edit.WindowsParallelBuilds < n || *edit.WindowsParallelBuilds > 8*n) {
			return domain.Invalidf("windows_parallel_builds must be between %d and %d for %d approved Windows hosts", n, 8*n, n)
		}
		for i := range hosts {
			hostEdit := domain.BuildHostEdit{Enabled: edit.WindowsRunnerEnabled}
			if edit.WindowsParallelBuilds != nil {
				count := *edit.WindowsParallelBuilds / n
				if i < *edit.WindowsParallelBuilds%n {
					count++
				}
				hostEdit.MaxParallel = &count
			}
			before := hosts[i]
			if err := hosts[i].ApplyEdit(hostEdit); err != nil {
				return err
			}
			if before.Enabled != hosts[i].Enabled || before.MaxParallel != hosts[i].MaxParallel {
				if err := r.BuildHosts().Update(ctx, hosts[i]); err != nil {
					return err
				}
				changed = append(changed, hosts[i])
			}
		}
		if err := r.Settings().Save(ctx, settings, actor, s.clock.Now()); err != nil {
			return err
		}
		settings = projectWindowsSettings(settings, hosts)
		return nil
	})
	if err != nil {
		return domain.BoardSettings{}, err
	}
	for _, h := range changed {
		slog.InfoContext(ctx, "build_host.updated", "actor_id", actor.ID, "actor_name", actor.Name, "event_time", s.clock.Now(), "host_id", h.ID, "enabled", h.Enabled, "max_parallel", h.MaxParallel)
	}
	return settings, nil
}
