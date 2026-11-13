package api

import (
	"cmp"
	"context"
	"net/http"
	"slices"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/jackc/pgx/v5"
)

// progress is the progress row of one learner for one card. The version 0 means that the learner has no row.
type progress struct {
	correct, incorrect             int
	lastCorrectAt, lastIncorrectAt *time.Time
	version                        int
}

// waits are the waits after a correct answer for the scores 0 to 6.
var waits = [...]time.Duration{4 * time.Hour, 24 * time.Hour, 72 * time.Hour, 168 * time.Hour, 336 * time.Hour, 720 * time.Hour, 1440 * time.Hour}

// dueAt is the due time of a row with a version above 0. A miss is the last result when its time is not earlier than the time of the last correct answer.
func (p progress) dueAt() time.Time {
	if p.lastIncorrectAt != nil && (p.lastCorrectAt == nil || !p.lastCorrectAt.After(*p.lastIncorrectAt)) {
		return p.lastIncorrectAt.Add(10 * time.Minute)
	}
	return p.lastCorrectAt.Add(waits[min(max(p.correct-2*p.incorrect, 0), len(waits)-1)])
}

// studyRow is a translation card with the progress of the learner.
type studyRow struct {
	card      Card
	createdAt time.Time
	progress  progress
}

// studyQueue gives the first 10 cards that are due at now, and the earliest due time that is later than now.
func studyQueue(rows []studyRow, now time.Time) StudyQueue {
	type dueRow struct {
		studyRow
		at time.Time
	}
	var q StudyQueue
	var due []dueRow
	for _, r := range rows {
		at := r.createdAt
		if r.progress.version > 0 {
			at = r.progress.dueAt()
			if at.After(now) {
				if q.NextDueAt == nil || at.Before(*q.NextDueAt) {
					q.NextDueAt = &at
				}
				continue
			}
		}
		due = append(due, dueRow{r, at})
	}
	slices.SortFunc(due, func(a, b dueRow) int {
		// The higher weight (incorrect+1)/(correct+1) first, compared as fractions with whole numbers.
		wa := (a.progress.incorrect + 1) * (b.progress.correct + 1)
		wb := (b.progress.incorrect + 1) * (a.progress.correct + 1)
		return cmp.Or(cmp.Compare(wb, wa), a.at.Compare(b.at), cmp.Compare(a.card.Id, b.card.Id))
	})
	for _, r := range due[:min(len(due), 10)] {
		q.Cards = append(q.Cards, StudyCard{Card: r.card, ProgressVersion: r.progress.version})
	}
	return q
}

// getStudyQueue answers the study queue of the signed-in learner in the group in the path.
func (s *Server) getStudyQueue(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	sub, fail := s.member(ctx, req)
	if fail != nil {
		return *fail
	}
	// A new card has no progress row, so the join gives NULL in each progress column, and pgx refuses NULL for an int.
	rows, err := s.DB.Query(ctx, "SELECT "+cardColumns+", created_at, COALESCE(correct, 0), COALESCE(incorrect, 0), last_correct_at, last_incorrect_at, COALESCE(version, 0) "+
		"FROM cards LEFT JOIN card_progress ON card_progress.card_id = cards.id AND card_progress.user_id = $2 "+
		"WHERE group_id = $1 AND NOT archived AND kind = 'translation'",
		req.PathParameters["groupId"], sub)
	if err != nil {
		return serverError(ctx, req, sub, err)
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (studyRow, error) {
		var r studyRow
		p := &r.progress
		var err error
		r.card, err = scanCard(row, &r.createdAt, &p.correct, &p.incorrect, &p.lastCorrectAt, &p.lastIncorrectAt, &p.version)
		return r, err
	})
	if err != nil {
		return serverError(ctx, req, sub, err)
	}
	return jsonResponse(http.StatusOK, studyQueue(list, time.Now()))
}
