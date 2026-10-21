package domain

// Owner is the party expected to act next on a ticket.
type Owner string

// The possible owners.
const (
	OwnerAgent  Owner = "agent"
	OwnerHuman  Owner = "human"
	OwnerNobody Owner = "nobody"
)
