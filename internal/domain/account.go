package domain

import "strings"

// Account pins a ticket to one model account. Workers only claim tickets open
// to their own account, so work one model cannot do waits for the one that
// can (2026-11-23, Ryan: Claude only writes code; art generation needs GPT,
// which the board runs as its Codex account).
type Account string

const (
	// AccountAny leaves the ticket to whichever account claims it first.
	AccountAny Account = ""
	// AccountCodex is the OpenAI account: code, plus image generation.
	AccountCodex Account = "codex"
	// AccountClaude is the Anthropic account: code only.
	AccountClaude Account = "claude"
)

// ParseAccount validates an account pin. "", "any" and "auto" mean no pin.
func ParseAccount(raw string) (Account, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "any", "auto":
		return AccountAny, nil
	case "codex", "gpt", "openai":
		return AccountCodex, nil
	case "claude", "anthropic":
		return AccountClaude, nil
	}
	return AccountAny, Invalidf("account must be any, codex or claude, not %q", raw)
}

// Label is the account's name as people read it.
func (a Account) Label() string {
	switch a {
	case AccountCodex:
		return "Codex only"
	case AccountClaude:
		return "Claude only"
	}
	return "Any account"
}

// OpenTo reports whether a worker on the given account may take the ticket.
// A claim that names no account takes only unpinned tickets.
func (a Account) OpenTo(worker Account) bool {
	return a == AccountAny || a == worker
}

// AccountCapabilities says what each account can do. The board manager and
// refinement read it to decide when a ticket needs a pin.
const AccountCapabilities = "Codex (OpenAI GPT): writes code and can generate images (art, textures, icons). " +
	"Claude (Anthropic): writes code; cannot generate images. " +
	"Pin a ticket to codex when its deliverable includes generated images or other GPT-only output; " +
	"pin to claude only when the owner asks. Leave every other ticket unpinned."
