package app_test

import (
	"strconv"
	"testing"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

type links struct{}

func (links) Ticket(id int64) string { return "https://board.example.com/#/t/" + strconv.FormatInt(id, 10) }

func policy() app.NotificationPolicy {
	return app.NewNotificationPolicy(links{}, app.DefaultNotificationRules()...)
}

func TestQuestionsNotifyLoudlyWithADeepLink(t *testing.T) {
	notification, isWanted := policy().Evaluate(app.Event{Type: domain.TimelineQuestions, TicketID: 7, Actor: agent, Detail: "Need a repo"})
	if !isWanted || notification.Priority != app.PriorityHigh || notification.Message != "Need a repo" {
		t.Fatalf("got %+v wanted=%v", notification, isWanted)
	}
	if notification.ClickURL != "https://board.example.com/#/t/7" {
		t.Fatalf("click URL %q", notification.ClickURL)
	}
}

func TestReviewNotificationOffersThePR(t *testing.T) {
	event := app.Event{Type: domain.TimelineMoved, Lane: domain.LaneReview, TicketID: 3, Actor: agent, PRURL: "https://git.example.com/pr/1"}
	notification, isWanted := policy().Evaluate(event)
	if !isWanted || len(notification.Actions) != 1 || notification.Actions[0].URL != event.PRURL {
		t.Fatalf("got %+v wanted=%v", notification, isWanted)
	}
}

func TestOwnActionsAndQuietEventsDoNotNotify(t *testing.T) {
	cases := map[string]app.Event{
		"person moves to review": {Type: domain.TimelineMoved, Lane: domain.LaneReview, Actor: owner},
		"agent comments":         {Type: domain.TimelineComment, Actor: agent},
		"agent moves to ready":   {Type: domain.TimelineMoved, Lane: domain.LaneReady, Actor: agent},
	}
	for name, event := range cases {
		t.Run(name, func(t *testing.T) {
			if _, isWanted := policy().Evaluate(event); isWanted {
				t.Fatal("notified")
			}
		})
	}
}
