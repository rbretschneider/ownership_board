package app_test

import (
	"slices"
	"testing"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

func TestNewTicketsQueueAtTheBottomOfTodo(t *testing.T) {
	f := newFixture(t)
	first, second := f.ticket("first", 0), f.ticket("second", 0)
	if got := f.laneOrder(domain.LaneTodo); !slices.Equal(got, []int64{first, second}) {
		t.Fatalf("order = %v", got)
	}
}

func TestMoveAfterPlacesTicketBelowItsNeighbour(t *testing.T) {
	f := newFixture(t)
	a, b, c := f.ticket("a", 0), f.ticket("b", 0), f.ticket("c", 0)
	f.move(owner, c, domain.LaneTodo, int64Ptr(a))
	if got := f.laneOrder(domain.LaneTodo); !slices.Equal(got, []int64{a, c, b}) {
		t.Fatalf("after placing c below a: %v", got)
	}
	f.move(owner, b, domain.LaneTodo, nil)
	if got := f.laneOrder(domain.LaneTodo); !slices.Equal(got, []int64{b, a, c}) {
		t.Fatalf("after moving b to the top: %v", got)
	}
}

func TestManyMovesBetweenTheSameNeighboursStayOrdered(t *testing.T) {
	f := newFixture(t)
	top, bottom := f.ticket("top", 0), f.ticket("bottom", 0)
	inserted := []int64{}
	for i := 0; i < 60; i++ {
		id := f.ticket("squeezed", 0)
		f.move(owner, id, domain.LaneTodo, int64Ptr(top))
		inserted = append([]int64{id}, inserted...)
	}
	want := append(append([]int64{top}, inserted...), bottom)
	if got := f.laneOrder(domain.LaneTodo); !slices.Equal(got, want) {
		t.Fatalf("order after repeated squeezing is wrong:\n got %v\nwant %v", got, want)
	}
}

func TestMoveAfterTicketInAnotherLaneIsRejected(t *testing.T) {
	f := newFixture(t)
	a, b := f.ticket("a", 0), f.ticket("b", 0)
	_, err := f.board.Move(f.ctx, owner, a, app.MoveInput{Lane: string(domain.LaneInProgress), AfterID: int64Ptr(b)})
	requireErrorKind(t, err, domain.KindInvalid)
}

func TestAskingKeepsTheStageAndHandsTheBallToTheOwner(t *testing.T) {
	f := newFixture(t)
	id := f.ticket("vague idea", 0)
	detail, err := f.agentAsk(id, ask("Mobile too?", "Dark by default?"))
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	ticket := detail.Ticket
	if ticket.Stage != domain.StageRefine || detail.OpenQuestions != 2 || ticket.Lease != nil || !ticket.NeedsOwner(detail.OpenQuestions) {
		t.Fatalf("stage %s open %d lease %v", ticket.Stage, detail.OpenQuestions, ticket.Lease)
	}
}

func TestAnsweringEverythingMakesTheTicketClaimableAgain(t *testing.T) {
	f := newFixture(t)
	id := f.ticket("vague idea", 0)
	asked, err := f.agentAsk(id, ask("One?", "Two?"))
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	first, second := asked.Questions[0].ID, asked.Questions[1].ID
	if _, err := f.board.Answer(f.ctx, owner, id, []domain.AnswerDraft{{QuestionID: first, Text: "yes"}}); err != nil {
		t.Fatalf("first answer: %v", err)
	}
	if _, isClaimed := f.claim(other, app.ClaimInput{}); isClaimed {
		t.Fatal("claimed a ticket with an open question")
	}
	if _, err := f.board.Answer(f.ctx, owner, id, []domain.AnswerDraft{{QuestionID: second, IsDeferred: true}}); err != nil {
		t.Fatalf("second answer: %v", err)
	}
	f.requireStage(id, domain.StageRefine)
	if work := f.mustClaim(other, app.ClaimInput{}); work.Ticket.Ticket.ID != id {
		t.Fatalf("claimed #%d after answering", work.Ticket.Ticket.ID)
	}
}

func TestSecondRoundIsNumberedAfterTheFirst(t *testing.T) {
	f := newFixture(t)
	id := f.ticket("idea", 0)
	first, err := f.agentAsk(id, ask("One?"))
	if err != nil {
		t.Fatalf("first ask: %v", err)
	}
	if _, err := f.board.Answer(f.ctx, owner, id, []domain.AnswerDraft{{QuestionID: first.Questions[0].ID, Text: "yes"}}); err != nil {
		t.Fatalf("answer: %v", err)
	}
	detail, err := f.agentAsk(id, ask("Two?"))
	if err != nil {
		t.Fatalf("second ask: %v", err)
	}
	if rounds := []int{detail.Questions[0].Round, detail.Questions[1].Round}; !slices.Equal(rounds, []int{1, 2}) {
		t.Fatalf("rounds = %v", rounds)
	}
}

func TestOnlyPeopleAnswer(t *testing.T) {
	f := newFixture(t)
	id := f.ticket("idea", 0)
	asked, _ := f.agentAsk(id, ask("One?"))
	_, err := f.board.Answer(f.ctx, agent, id, []domain.AnswerDraft{{QuestionID: asked.Questions[0].ID, Text: "me"}})
	requireErrorKind(t, err, domain.KindForbidden)
}

func TestAnswerForAnotherTicketsQuestionIsRejected(t *testing.T) {
	f := newFixture(t)
	a, b := f.ticket("a", 0), f.ticket("b", 0)
	asked, _ := f.agentAsk(a, ask("One?"))
	_, err := f.board.Answer(f.ctx, owner, b, []domain.AnswerDraft{{QuestionID: asked.Questions[0].ID, Text: "x"}})
	requireErrorKind(t, err, domain.KindInvalid)
}

func TestAgentCannotSubmitASpecWithoutARepo(t *testing.T) {
	f := newFixture(t)
	id := f.ticket("no repo", 0)
	f.mustClaim(agent, app.ClaimInput{})
	_, err := f.board.SaveSpec(f.ctx, agent, id, app.SaveTextInput{Text: "Spec.", IsSubmit: true})
	requireErrorKind(t, err, domain.KindInvalid)
}

func TestAgentsOnlyMoveAFinishedBuildToReview(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	id := f.ticket("skip ahead", api.ID)
	f.mustClaim(agent, app.ClaimInput{})
	_, err := f.board.Move(f.ctx, agent, id, app.MoveInput{Lane: string(domain.LaneInProgress)})
	requireErrorKind(t, err, domain.KindForbidden)
	_, err = f.board.Move(f.ctx, agent, id, app.MoveInput{Lane: string(domain.LaneReview)})
	requireErrorKind(t, err, domain.KindConflict)
}

func TestPersonDropLandsOnTheFirstUnmetCheckpoint(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	id := f.ticket("spec it", api.ID)
	if _, err := f.board.SaveSpec(f.ctx, owner, id, app.SaveTextInput{Text: "Owner's spec."}); err != nil {
		t.Fatalf("spec: %v", err)
	}
	f.move(owner, id, domain.LaneInProgress, nil)
	f.move(owner, id, domain.LaneTodo, nil)
	f.requireStage(id, domain.StageRefine)
}

func TestAgentEditsAreLimitedToTheirStage(t *testing.T) {
	f := newFixture(t)
	id := f.ticket("refining", 0)
	f.mustClaim(agent, app.ClaimInput{})
	pr := "https://example.com/pr/1"
	_, err := f.board.Edit(f.ctx, agent, id, domain.TicketEdit{PRURL: &pr})
	requireErrorKind(t, err, domain.KindForbidden)
}

func TestCreateWithUnknownRepoIsRejected(t *testing.T) {
	f := newFixture(t)
	_, err := f.board.Create(f.ctx, owner, domain.TicketDraft{Kind: "bug", Title: "x", RepoID: 99})
	requireErrorKind(t, err, domain.KindNotFound)
}

func TestBoardScopes(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	assigned, unassigned := f.ticket("assigned", api.ID), f.ticket("loose", 0)
	cases := map[string]struct {
		scope app.RepoScope
		want  []int64
	}{
		"all":        {app.RepoScope{}, []int64{assigned, unassigned}},
		"one repo":   {app.RepoScope{RepoID: api.ID}, []int64{assigned}},
		"unassigned": {app.RepoScope{IsUnassigned: true}, []int64{unassigned}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			board, err := f.board.Board(f.ctx, tc.scope)
			if err != nil {
				t.Fatalf("Board: %v", err)
			}
			var got []int64
			for _, summary := range board.Tickets {
				got = append(got, summary.Ticket.ID)
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEventsArePublishedAfterChanges(t *testing.T) {
	f := newFixture(t)
	id := f.ticket("idea", 0)
	f.move(owner, id, domain.LaneInProgress, nil)
	if err := f.board.Delete(f.ctx, owner, id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	want := []domain.TimelineType{domain.TimelineCreated, domain.TimelineStage, domain.TimelineMoved, domain.TimelineDeleted}
	if got := f.events.types(); !slices.Equal(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
}

func TestAgentCannotChangeATicketItDoesNotHold(t *testing.T) {
	f := newFixture(t)
	id := f.ticket("someone else's", 0)
	title := "rewritten"
	_, editErr := f.board.Edit(f.ctx, agent, id, domain.TicketEdit{Title: &title})
	_, specErr := f.board.SaveSpec(f.ctx, agent, id, app.SaveTextInput{Text: "new spec"})
	_, commentErr := f.board.Comment(f.ctx, agent, id, "hello")
	_, moveErr := f.board.Move(f.ctx, agent, id, app.MoveInput{Lane: string(domain.LaneReview)})
	for _, err := range []error{editErr, specErr, commentErr, moveErr} {
		requireErrorKind(t, err, domain.KindForbidden)
	}
}
