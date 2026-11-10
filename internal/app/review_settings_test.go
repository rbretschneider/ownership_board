package app_test

import (
	"testing"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

func TestReviewSettingsDefaultToAFreshAdversarialReviewByAnotherModel(t *testing.T) {
	f := newFixture(t)
	settings, err := f.board.Settings(f.ctx)
	if err != nil {
		t.Fatalf("settings: %v", err)
	}
	if settings.ReviewBy != domain.ReviewByOtherModel || !settings.ReviewFreshContext || !settings.ReviewAdversarial {
		t.Fatalf("defaults %q fresh %v adversarial %v", settings.ReviewBy, settings.ReviewFreshContext, settings.ReviewAdversarial)
	}

	by, off := domain.ReviewByClaude, false
	if _, err := f.board.UpdateSettings(f.ctx, owner, domain.SettingsEdit{ReviewBy: &by, ReviewAdversarial: &off}); err != nil {
		t.Fatalf("update: %v", err)
	}
	settings, err = f.board.Settings(f.ctx)
	if err != nil {
		t.Fatalf("settings: %v", err)
	}
	if settings.ReviewBy != domain.ReviewByClaude || !settings.ReviewFreshContext || settings.ReviewAdversarial {
		t.Fatalf("after edit %q fresh %v adversarial %v", settings.ReviewBy, settings.ReviewFreshContext, settings.ReviewAdversarial)
	}

	bad := domain.ReviewBy("gemini")
	_, err = f.board.UpdateSettings(f.ctx, owner, domain.SettingsEdit{ReviewBy: &bad})
	requireErrorKind(t, err, domain.KindInvalid)
	_, err = f.board.UpdateSettings(f.ctx, agent, domain.SettingsEdit{ReviewBy: &by})
	requireErrorKind(t, err, domain.KindForbidden)
}

func TestEachRepoChoosesWhichReviewFindingsBlockPublication(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	if api.ReviewBlocking != domain.ReviewBlocksP1 {
		t.Fatalf("new repo blocks at %q, want p1", api.ReviewBlocking)
	}
	nits := domain.ReviewBlocksP3
	repo, err := f.repos.Restyle(f.ctx, owner, api.ID, domain.RepoStyle{ReviewBlocking: &nits})
	if err != nil || repo.ReviewBlocking != domain.ReviewBlocksP3 {
		t.Fatalf("restyle: %q, %v", repo.ReviewBlocking, err)
	}
	other := f.repo("acme/web")
	if other.ReviewBlocking != domain.ReviewBlocksP1 {
		t.Fatalf("another repo changed too: %q", other.ReviewBlocking)
	}
	bad := domain.ReviewBlocking("p9")
	_, err = f.repos.Restyle(f.ctx, owner, api.ID, domain.RepoStyle{ReviewBlocking: &bad})
	requireErrorKind(t, err, domain.KindInvalid)
}
