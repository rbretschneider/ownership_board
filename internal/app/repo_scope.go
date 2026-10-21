package app

// RepoScope limits a board view to one repo, or to tickets without one. The
// zero value shows everything.
type RepoScope struct {
	RepoID       int64
	IsUnassigned bool
}

func (s RepoScope) filter() TicketFilter {
	switch {
	case s.IsUnassigned:
		return TicketFilter{IsScoped: true, IncludeUnassigned: true}
	case s.RepoID > 0:
		return TicketFilter{IsScoped: true, RepoIDs: []int64{s.RepoID}}
	default:
		return TicketFilter{}
	}
}
