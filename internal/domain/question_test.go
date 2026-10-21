package domain_test

import (
	"testing"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

func intPtr(v int) *int { return &v }

func mustQuestion(t *testing.T, options ...string) domain.Question {
	t.Helper()
	question, err := domain.NewQuestion(domain.QuestionDraft{Prompt: "Which repo?", Options: options}, 1, 0, human, now)
	if err != nil {
		t.Fatalf("NewQuestion: %v", err)
	}
	return question
}

func TestNewQuestionRejectsRecommendationOutsideOptions(t *testing.T) {
	_, err := domain.NewQuestion(domain.QuestionDraft{Prompt: "Pick", Options: []string{"a"}, Recommended: intPtr(1)}, 1, 0, human, now)
	requireKind(t, err, domain.KindInvalid)
}

func TestAnswerNeedsChoiceTextOrDeferral(t *testing.T) {
	question := mustQuestion(t, "a", "b")
	requireKind(t, question.ApplyAnswer(domain.AnswerDraft{}, human, now), domain.KindInvalid)
	requireKind(t, question.ApplyAnswer(domain.AnswerDraft{Choice: intPtr(2)}, human, now), domain.KindInvalid)
	if !question.IsOpen() {
		t.Fatal("a rejected answer closed the question")
	}
}

func TestDeferredAnswerCloses(t *testing.T) {
	question := mustQuestion(t)
	if err := question.ApplyAnswer(domain.AnswerDraft{IsDeferred: true}, human, now); err != nil {
		t.Fatalf("ApplyAnswer: %v", err)
	}
	if question.IsOpen() || !question.Answer.IsDeferred {
		t.Fatalf("answer = %+v", question.Answer)
	}
}

func TestAnsweredQuestionCannotBeAnsweredAgain(t *testing.T) {
	question := mustQuestion(t, "a")
	if err := question.ApplyAnswer(domain.AnswerDraft{Choice: intPtr(0)}, human, now); err != nil {
		t.Fatalf("ApplyAnswer: %v", err)
	}
	requireKind(t, question.ApplyAnswer(domain.AnswerDraft{Text: "again"}, human, now), domain.KindConflict)
}
