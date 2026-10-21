package domain

// Provider is where a repo's listing comes from.
type Provider string

// The providers.
const (
	ProviderGitHub    Provider = "github"
	ProviderBitbucket Provider = "bitbucket"
	ProviderForgejo   Provider = "forgejo"
	ProviderManual    Provider = "manual"
)
