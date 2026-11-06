package app

import (
	"context"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// Long-poll limits for claiming work.
const (
	MaxClaimWaitSeconds = 600
	// claimRecheckInterval bounds how long a waiting claim sleeps without a
	// board change, so an expired lease is noticed without any event.
	claimRecheckInterval = 30 * time.Second
)

// AgentService holds the use cases only agents perform: claiming work and
// keeping or giving up a lease.
type AgentService struct {
	store    Store
	clock    Clock
	doorbell Doorbell
	writer   ticketWriter
}

// ClaimInput is a request for the next piece of work.
type ClaimInput struct {
	// Repo, when set, is the full name of the repo the agent works in. It then
	// claims that repo's tickets plus unassigned ones that still need
	// refining, since working out the repo is part of refinement.
	Repo         string
	WaitSeconds  int
	LeaseMinutes int
}

// NewAgentService wires the service to its dependencies.
func NewAgentService(store Store, clock Clock, sink EventSink, doorbell Doorbell) *AgentService {
	return &AgentService{store: store, clock: clock, doorbell: doorbell, writer: ticketWriter{store: store, sink: sink, clock: clock}}
}

// Claim leases the most urgent claimable ticket to the agent. With a wait, it
// blocks until work appears or the wait runs out. It reports false when there
// is nothing to do.
func (s *AgentService) Claim(ctx context.Context, actor domain.Actor, input ClaimInput) (Work, bool, error) {
	request, err := s.claimRequest(ctx, actor, input)
	if err != nil {
		return Work{}, false, err
	}
	listener := s.doorbell.Listen()
	defer listener.Close()
	deadline := s.clock.Now().Add(time.Duration(input.WaitSeconds) * time.Second)
	for {
		work, isClaimed, err := s.tryClaim(ctx, actor, request)
		if err != nil || isClaimed {
			return work, isClaimed, err
		}
		remaining := deadline.Sub(s.clock.Now())
		if remaining <= 0 || ctx.Err() != nil {
			return Work{}, false, nil
		}
		listener.Wait(ctx, min(remaining, claimRecheckInterval))
	}
}

// Heartbeat extends the agent's lease on a ticket it holds.
func (s *AgentService) Heartbeat(ctx context.Context, actor domain.Actor, id int64, leaseMinutes int) (TicketDetail, error) {
	duration, err := domain.LeaseDuration(leaseMinutes)
	if err != nil {
		return TicketDetail{}, err
	}
	return s.writer.change(ctx, actor, id, func(_ context.Context, _ Repos, ticket *domain.Ticket) ([]domain.TimelineEntry, error) {
		if err := requireHolder(*ticket, actor, s.clock.Now()); err != nil {
			return nil, err
		}
		if !ticket.Stage.IsAgentStage() || ticket.IsBlocked() {
			return nil, domain.Conflictf("ticket %d is no longer with an agent", ticket.ID)
		}
		return nil, ticket.Claim(actor.Name, duration, s.clock.Now())
	})
}

// Release gives up the agent's lease so another session may claim the ticket.
func (s *AgentService) Release(ctx context.Context, actor domain.Actor, id int64) (TicketDetail, error) {
	return s.writer.change(ctx, actor, id, func(_ context.Context, _ Repos, ticket *domain.Ticket) ([]domain.TimelineEntry, error) {
		if err := requireHolder(*ticket, actor, s.clock.Now()); err != nil {
			return nil, err
		}
		ticket.Release(s.clock.Now())
		entry := domain.NewEntry(ticket.ID, domain.TimelineReleased, actor, ticket.UpdatedAt)
		return []domain.TimelineEntry{entry}, nil
	})
}

// Brief renders the brief for a ticket without claiming it.
func (s *AgentService) Brief(ctx context.Context, id int64) (string, error) {
	var brief string
	err := s.store.Read(ctx, func(repos Repos) error {
		ticket, err := repos.Tickets().Get(ctx, id)
		if err != nil {
			return err
		}
		detail, err := loadDetail(ctx, repos, ticket)
		if err != nil {
			return err
		}
		input, err := briefInput(ctx, repos, detail, "", s.clock.Now())
		brief = RenderBrief(input)
		return err
	})
	return brief, err
}

// claimRequest is a validated ClaimInput.
type claimRequest struct {
	filter TicketFilter
	lease  time.Duration
}

// filterFor widens a repo-scoped claim to unassigned tickets being refined.
func (r claimRequest) filterFor(stage domain.Stage) TicketFilter {
	filter := r.filter
	filter.IncludeUnassigned = filter.IsScoped && stage == domain.StageRefine
	return filter
}

func (s *AgentService) claimRequest(ctx context.Context, actor domain.Actor, input ClaimInput) (claimRequest, error) {
	if !actor.IsAgent() {
		return claimRequest{}, domain.Forbiddenf("only an agent token can claim work")
	}
	if input.WaitSeconds < 0 || input.WaitSeconds > MaxClaimWaitSeconds {
		return claimRequest{}, domain.Invalidf("wait_seconds must be between 0 and %d", MaxClaimWaitSeconds)
	}
	lease, err := domain.LeaseDuration(input.LeaseMinutes)
	if err != nil {
		return claimRequest{}, err
	}
	filter, err := s.repoFilter(ctx, input.Repo)
	return claimRequest{filter: filter, lease: lease}, err
}

func (s *AgentService) repoFilter(ctx context.Context, fullName string) (TicketFilter, error) {
	if fullName == "" {
		return TicketFilter{}, nil
	}
	var matches []domain.Repo
	err := s.store.Read(ctx, func(repos Repos) error {
		var err error
		matches, err = repos.CodeRepos().ByFullName(ctx, fullName)
		return err
	})
	if err != nil {
		return TicketFilter{}, err
	}
	if len(matches) == 0 {
		return TicketFilter{}, domain.NotFoundf("repo %q is not on the board; sync repos or add it in Settings", fullName)
	}
	filter := TicketFilter{IsScoped: true}
	for _, repo := range matches {
		filter.RepoIDs = append(filter.RepoIDs, repo.ID)
	}
	return filter, nil
}

func (s *AgentService) tryClaim(ctx context.Context, actor domain.Actor, request claimRequest) (Work, bool, error) {
	var work Work
	var events []Event
	isClaimed := false
	err := s.store.Write(ctx, func(repos Repos) error {
		ticket, isFound, err := s.nextClaimable(ctx, repos, request)
		if err != nil || !isFound {
			return err
		}
		action, _, _ := domain.ActionFor(ticket.Stage)
		var detail TicketDetail
		detail, events, err = s.writer.apply(ctx, repos, &ticket, s.claimChange(actor, request.lease))
		if err != nil {
			return err
		}
		input, err := briefInput(ctx, repos, detail, action, s.clock.Now())
		work = Work{Action: action, Ticket: detail, Repo: input.Repo, Brief: RenderBrief(input)}
		isClaimed = true
		return err
	})
	if err != nil {
		return Work{}, false, err
	}
	s.writer.sink.Publish(ctx, events)
	return work, isClaimed, nil
}

// nextClaimable runs inside the claim's write transaction, so the in-flight
// count and the claim cannot race another claim.
func (s *AgentService) nextClaimable(ctx context.Context, repos Repos, request claimRequest) (domain.Ticket, bool, error) {
	settings, err := repos.Settings().Get(ctx)
	if err != nil {
		return domain.Ticket{}, false, err
	}
	for _, stage := range domain.ClaimOrder {
		filter := request.filterFor(stage)
		filter.ExcludeWindowsWaiting = !settings.WindowsRunnerEnabled && stage == domain.StageBuild
		if stage == domain.StageReady {
			isFull, err := autonomousSlotsFull(ctx, repos)
			if err != nil {
				return domain.Ticket{}, false, err
			}
			filter.ExcludeAutonomous = isFull
		}
		for skipped := 0; ; skipped++ {
			ticket, isFound, err := repos.Tickets().NextClaimable(ctx, stage, filter, s.clock.Now())
			if err != nil || !isFound {
				if err != nil {
					return domain.Ticket{}, false, err
				}
				break
			}
			if stage != domain.StageBuild || skipped >= maxOverlapSkips {
				return ticket, true, nil
			}
			held, err := heldByOverlap(ctx, repos, ticket)
			if err != nil || !held {
				return ticket, err == nil, err
			}
			filter.ExcludeIDs = append(filter.ExcludeIDs, ticket.ID)
		}
	}
	return domain.Ticket{}, false, nil
}

// maxOverlapSkips bounds how many held-back builds one claim looks past.
const maxOverlapSkips = 20

// heldByOverlap reports whether a build that has not started yet shares
// files with a build already under way in its repo: one with a branch, or
// an open pull request. The first build to start goes on; the other waits
// until it merges, so the two never conflict. A started build is never held
// back, so held tickets only ever wait on work that can finish.
func heldByOverlap(ctx context.Context, repos Repos, ticket domain.Ticket) (bool, error) {
	if ticket.RepoID == 0 || ticket.Branch != "" || ticket.PRURL != "" {
		return false, nil
	}
	mine := domain.DesignFiles(ticket.Design)
	if len(mine) == 0 {
		return false, nil
	}
	all, err := repos.Tickets().List(ctx, TicketFilter{IsScoped: true, RepoIDs: []int64{ticket.RepoID}})
	if err != nil {
		return false, err
	}
	var designs []map[string]bool
	var underWay []map[string]bool
	for _, other := range all {
		if other.Design == "" || other.RepoID != ticket.RepoID {
			continue
		}
		files := domain.DesignFiles(other.Design)
		designs = append(designs, files)
		started := (other.Stage == domain.StageBuild && other.Branch != "") || other.Stage == domain.StagePR
		if started && other.ID != ticket.ID {
			underWay = append(underWay, files)
		}
	}
	hubs := domain.HubFiles(designs)
	for _, files := range underWay {
		if len(domain.SharedFiles(mine, files, hubs)) > 0 {
			return true, nil
		}
	}
	return false, nil
}

// autonomousSlotsFull reports whether as many autonomous tickets as the
// board allows are already in design or build. Only a claim from Ready puts
// an autonomous ticket in flight, so only that claim is held back; one
// already in flight can always be picked up again.
func autonomousSlotsFull(ctx context.Context, repos Repos) (bool, error) {
	settings, err := repos.Settings().Get(ctx)
	if err != nil {
		return false, err
	}
	inFlight, err := repos.Tickets().CountInFlightAutonomous(ctx)
	if err != nil {
		return false, err
	}
	return inFlight >= settings.AutonomousInFlightCap, nil
}

// claimChange moves the ticket to the stage its action works in, then leases
// it. The move comes first because a stage change drops any lease.
func (s *AgentService) claimChange(actor domain.Actor, lease time.Duration) ticketChange {
	return func(ctx context.Context, repos Repos, ticket *domain.Ticket) ([]domain.TimelineEntry, error) {
		action, workStage, _ := domain.ActionFor(ticket.Stage)
		var entries []domain.TimelineEntry
		if workStage != ticket.Stage {
			moved, err := changeStage(ctx, repos, ticket, workStage, actor, s.clock)
			if err != nil {
				return nil, err
			}
			entries = moved
		}
		if err := ticket.Claim(actor.Name, lease, s.clock.Now()); err != nil {
			return nil, err
		}
		claimed := domain.NewEntry(ticket.ID, domain.TimelineClaimed, actor, ticket.UpdatedAt).With("action", string(action))
		return append([]domain.TimelineEntry{claimed}, entries...), nil
	}
}

// briefInput gathers what the brief needs: the ticket's repo, or every active
// repo when it has none so the agent can work out which one is meant.
func briefInput(ctx context.Context, repos Repos, detail TicketDetail, action domain.WorkAction, now time.Time) (BriefInput, error) {
	input := BriefInput{Detail: detail, Action: action, Now: now}
	if detail.Ticket.RepoID == 0 {
		known, err := repos.CodeRepos().List(ctx)
		input.KnownRepos = activeRepos(known)
		return input, err
	}
	repo, err := repos.CodeRepos().Get(ctx, detail.Ticket.RepoID)
	if err != nil {
		return BriefInput{}, err
	}
	input.Repo = &repo
	return input, nil
}

func activeRepos(all []domain.Repo) []domain.Repo {
	active := make([]domain.Repo, 0, len(all))
	for _, repo := range all {
		if !repo.IsArchived && !repo.IsMissing() {
			active = append(active, repo)
		}
	}
	return active
}

func requireHolder(ticket domain.Ticket, actor domain.Actor, now time.Time) error {
	if !actor.IsAgent() {
		return domain.Forbiddenf("only an agent token holds leases")
	}
	if !ticket.IsLeased(now) || ticket.Lease.Holder != actor.Name {
		return domain.Conflictf("%s does not hold the lease on ticket %d; claim it again", actor.Name, ticket.ID)
	}
	return nil
}

// SetWorkModel records the actual worker model, independently of the coordinator token.
func (s *AgentService) SetWorkModel(ctx context.Context, actor domain.Actor, id int64, model string) (TicketDetail, error) {
	text, err := domain.RequireText("model", model, 120)
	if err != nil {
		return TicketDetail{}, err
	}
	if err := rejectHiddenFromAgent(actor, map[string]string{"model": text}); err != nil {
		return TicketDetail{}, err
	}
	return s.writer.change(ctx, actor, id, func(_ context.Context, _ Repos, ticket *domain.Ticket) ([]domain.TimelineEntry, error) {
		if err := requireHolder(*ticket, actor, s.clock.Now()); err != nil {
			return nil, err
		}
		if ticket.WorkModel == text {
			return nil, nil
		}
		ticket.WorkModel = text
		ticket.UpdatedAt = s.clock.Now()
		entry := domain.NewEntry(ticket.ID, domain.TimelineEdited, actor, ticket.UpdatedAt).With("fields", []string{"working model"}).With("model", text)
		return []domain.TimelineEntry{entry}, nil
	})
}
