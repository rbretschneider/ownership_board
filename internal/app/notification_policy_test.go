package app_test

import (
	"strconv"
	"testing"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

type links struct{}

func (links) Ticket(id int64) string {
	return "https://board.example.com/#/t/" + strconv.FormatInt(id, 10)
}

func policy() app.NotificationPolicy {
	return app.NewNotificationPolicy(links{}, app.DefaultNotificationRules()...)
}

func TestQuestionsNotifyLoudlyWithADeepLink(t *testing.T) {
	notification, isWanted := policy().Evaluate(app.Event{Type: domain.TimelineQuestions, TicketID: 7, Actor: agent, Detail: "Need a repo"})
	if !isWanted || notification.Priority != app.PriorityHigh || notification.Message == "Need a repo" {
		t.Fatalf("got %+v wanted=%v", notification, isWanted)
	}
	if notification.ClickURL != "https://board.example.com/#/t/7" {
		t.Fatalf("click URL %q", notification.ClickURL)
	}
}

func TestReviewNotificationOffersNoActions(t *testing.T) {
	event := app.Event{Type: domain.TimelineStage, Stage: domain.StagePR, TicketID: 3, Actor: agent, PRURL: "https://git.example.com/pr/1"}
	notification, isWanted := policy().Evaluate(event)
	if !isWanted || len(notification.Actions) != 0 {
		t.Fatalf("got %+v wanted=%v", notification, isWanted)
	}
}

func TestOwnActionsAndQuietEventsDoNotNotify(t *testing.T) {
	cases := map[string]app.Event{
		"person moves to review": {Type: domain.TimelineStage, Stage: domain.StagePR, Actor: owner},
		"agent comments":         {Type: domain.TimelineComment, Actor: agent},
		"agent starts a design":  {Type: domain.TimelineStage, Stage: domain.StageDesign, Actor: agent},
	}
	for name, event := range cases {
		t.Run(name, func(t *testing.T) {
			if _, isWanted := policy().Evaluate(event); isWanted {
				t.Fatal("notified")
			}
		})
	}
}

func TestFilingAnAutonomousTicketNotifiesTheOwner(t *testing.T) {
	filed := app.Event{Type: domain.TimelineCreated, TicketID: 4, Actor: delegate, IsAutonomous: true, Title: "agent words"}
	notification, isWanted := policy().Evaluate(filed)
	if !isWanted || notification.Title != "#4 filed as autonomous" {
		t.Fatalf("got %+v wanted=%v", notification, isWanted)
	}
	if _, isWanted := policy().Evaluate(app.Event{Type: domain.TimelineCreated, Actor: agent}); isWanted {
		t.Fatal("an ordinary agent-filed ticket notified")
	}
}
