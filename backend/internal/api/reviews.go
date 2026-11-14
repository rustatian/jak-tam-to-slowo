package api

import (
	"context"
	"encoding/json/v2"
	"errors"
	"net/http"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/aws/aws-lambda-go/events"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/rustatian/jak-tam-to-slowo/backend/internal/db"
)

var (
	errNotCurrent = errors.New("the progress version is not current")
	errOtherCard  = errors.New("the attempt ID is stored for another card")
)

// createReview grades an answer of the signed-in learner to a translation card of the group in the path, and counts
// it in the progress row of the learner for the card.
func (s *Server) createReview(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	sub, fail := s.member(ctx, req)
	if fail != nil {
		return *fail
	}
	var body ReviewCreate
	if err := json.Unmarshal([]byte(req.Body), &body); err != nil {
		return jsonResponse(http.StatusBadRequest, Error{Message: "invalid JSON body"})
	}
	var attempt pgtype.UUID
	if err := attempt.Scan(body.AttemptId); err != nil {
		return jsonResponse(http.StatusBadRequest, Error{Message: "attemptId must be a UUID"})
	}
	switch {
	case body.ProgressVersion < 0:
		return jsonResponse(http.StatusBadRequest, Error{Message: "progressVersion must be 0 or more"})
	case !body.Direction.Valid():
		return jsonResponse(http.StatusBadRequest, Error{Message: "direction must be ru-pl or pl-ru"})
	case utf8.RuneCountInString(body.Answer) > 500:
		return jsonResponse(http.StatusBadRequest, Error{Message: "answer must have at most 500 characters"})
	}
	now := time.Now()
	var review Review
	var status int
	// The reads and the writes use one transaction. When another request writes the progress row after the reads,
	// the transaction fails with SQLSTATE 40001 and db.Tx runs the function again. The second run answers 200 when
	// the other request stored the same attempt, and 409 when it had another attempt. So each run sets review and
	// status.
	err := db.Tx(ctx, s.DB, func(tx pgx.Tx) error {
		// A stored attempt gets its stored grade and counts nothing. The card ID is compared as text, as in the read
		// below.
		var stored Grade
		var sameCard bool
		err := tx.QueryRow(ctx, "SELECT grade, card_id::text = $3 FROM reviews WHERE user_id = $1 AND attempt_id = $2",
			sub, attempt, body.CardId).Scan(&stored, &sameCard)
		switch {
		case err == nil && !sameCard:
			return errOtherCard
		case err == nil:
			card, err := scanCard(tx.QueryRow(ctx, "SELECT "+cardColumns+" FROM cards WHERE group_id = $1 AND id::text = $2",
				req.PathParameters["groupId"], body.CardId))
			if err != nil {
				return err
			}
			review = Review{Grade: stored, Correction: grade(card, body.Direction, body.Answer).Correction}
			status = http.StatusOK
			return nil
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		}
		var version int
		// The ID is compared as text: PostgreSQL refuses a uuid parameter that is not a UUID (SQLSTATE 22P02).
		// https://www.postgresql.org/docs/16/errcodes-appendix.html
		// A card without a progress row of the learner has the version 0.
		card, err := scanCard(tx.QueryRow(ctx, "SELECT "+cardColumns+", COALESCE(version, 0) "+
			"FROM cards LEFT JOIN card_progress ON card_progress.card_id = cards.id AND card_progress.user_id = $3 "+
			"WHERE group_id = $1 AND cards.id::text = $2 AND NOT archived AND kind = 'translation'",
			req.PathParameters["groupId"], body.CardId, sub), &version)
		if err != nil {
			return err
		}
		if version != body.ProgressVersion {
			return errNotCurrent
		}
		review = grade(card, body.Direction, body.Answer)
		status = http.StatusCreated
		// add is the counts and the time of this answer. Each grade that is not correct counts as a miss.
		add := progress{incorrect: 1, lastIncorrectAt: &now}
		if review.Grade == Correct {
			add = progress{correct: 1, lastCorrectAt: &now}
		}
		// EXCLUDED is the row proposed for insertion, so the update adds the counts of this answer and keeps the time
		// that this answer does not set.
		// https://www.postgresql.org/docs/16/sql-insert.html#SQL-ON-CONFLICT
		_, err = tx.Exec(ctx, "INSERT INTO card_progress (user_id, card_id, correct, incorrect, last_correct_at, last_incorrect_at, version) "+
			"VALUES ($1, $2, $3, $4, $5, $6, 1) ON CONFLICT (card_id, user_id) DO UPDATE SET "+
			"correct = card_progress.correct + EXCLUDED.correct, incorrect = card_progress.incorrect + EXCLUDED.incorrect, "+
			"last_correct_at = COALESCE(EXCLUDED.last_correct_at, card_progress.last_correct_at), "+
			"last_incorrect_at = COALESCE(EXCLUDED.last_incorrect_at, card_progress.last_incorrect_at), "+
			"version = card_progress.version + 1",
			sub, card.Id, add.correct, add.incorrect, add.lastCorrectAt, add.lastIncorrectAt)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "INSERT INTO reviews (user_id, attempt_id, card_id, grade) VALUES ($1, $2, $3, $4)",
			sub, attempt, card.Id, review.Grade)
		return err
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return jsonResponse(http.StatusNotFound, Error{Message: "card not found"})
	case errors.Is(err, errNotCurrent):
		return jsonResponse(http.StatusConflict, Error{Message: "progressVersion is not current"})
	case errors.Is(err, errOtherCard):
		return jsonResponse(http.StatusConflict, Error{Message: "attemptId is used for another card"})
	case err != nil:
		return serverError(ctx, req, sub, err)
	}
	return jsonResponse(status, review)
}

// grade grades an answer to the card for the direction of the prompt. Only the terms of the answer side count.
func grade(card Card, direction Direction, answer string) Review {
	polish := direction == RuPl
	terms := append([]string{card.Ru}, card.AlternativesRu...)
	if polish {
		terms = append([]string{card.Pl}, card.AlternativesPl...)
	}
	key := answerKey(answer)
	if slices.ContainsFunc(terms, func(term string) bool { return answerKey(term) == key }) {
		return Review{Grade: Correct, Correction: terms[0]}
	}
	// A learner without a Polish keyboard types the nine Polish letters without their marks.
	if polish {
		key = searchKey(answer)
		if i := slices.IndexFunc(terms, func(term string) bool { return searchKey(term) == key }); i >= 0 {
			return Review{Grade: Almost, Correction: terms[i]}
		}
	}
	return Review{Grade: Again, Correction: terms[0]}
}
