package domain

import "strings"

// ReviewBy chooses who reviews an agent's finished work before it is
// published. It is a board-wide setting people change in Settings.
type ReviewBy string

const (
	// ReviewByOtherModel is the default and always offered: any model other
	// than the one that did the work, preferring the other provider when it
	// has usage left.
	ReviewByOtherModel ReviewBy = "other_model"
	// ReviewBySameModel reviews with the model that did the work.
	ReviewBySameModel ReviewBy = "same_model"
	// ReviewByClaude and ReviewByCodex pin the review to one account.
	ReviewByClaude ReviewBy = "claude"
	ReviewByCodex  ReviewBy = "codex"
)

// ReviewByChoices lists the options in the order Settings shows them.
var ReviewByChoices = []ReviewBy{ReviewByOtherModel, ReviewBySameModel, ReviewByClaude, ReviewByCodex}

// ParseReviewBy reads a stored or submitted reviewer choice.
func ParseReviewBy(raw string) (ReviewBy, error) {
	value := ReviewBy(strings.TrimSpace(raw))
	for _, choice := range ReviewByChoices {
		if value == choice {
			return value, nil
		}
	}
	return "", Invalidf("review_by must be one of other_model, same_model, claude or codex")
}

// ReviewBlocking is the lowest review finding priority that stops a
// publication in a repo. Findings below it are listed in the pull request
// as follow-ups instead.
type ReviewBlocking string

const (
	// ReviewBlocksP0 publishes unless a critical (P0) finding remains.
	ReviewBlocksP0 ReviewBlocking = "p0"
	// ReviewBlocksP1 is the default: must-fix findings (P0 and P1) block.
	ReviewBlocksP1 ReviewBlocking = "p1"
	// ReviewBlocksP2 also blocks should-fix findings.
	ReviewBlocksP2 ReviewBlocking = "p2"
	// ReviewBlocksP3 blocks every finding, nits included.
	ReviewBlocksP3 ReviewBlocking = "p3"
)

// DefaultReviewBlocking is the level of a new repo.
const DefaultReviewBlocking = ReviewBlocksP1

// ParseReviewBlocking reads a stored or submitted blocking level.
func ParseReviewBlocking(raw string) (ReviewBlocking, error) {
	switch value := ReviewBlocking(strings.ToLower(strings.TrimSpace(raw))); value {
	case ReviewBlocksP0, ReviewBlocksP1, ReviewBlocksP2, ReviewBlocksP3:
		return value, nil
	}
	return "", Invalidf("review_blocking must be p0, p1, p2 or p3")
}

// Stored keys of the review settings.
const (
	SettingReviewBy           = "review_by"
	SettingReviewFreshContext = "review_fresh_context"
	SettingReviewAdversarial  = "review_adversarial"
)
