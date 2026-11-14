package api

import (
	"context"
	"encoding/json/v2"
	"maps"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestGrade(t *testing.T) {
	slowo := Card{Pl: "Słowo", Ru: "Слово", AlternativesPl: []string{"Wyraz"}, AlternativesRu: []string{"словечко"}}
	zolw := Card{Pl: "Żółw", Ru: "Черепаха", AlternativesPl: []string{}, AlternativesRu: []string{}}
	tests := []struct {
		name           string
		card           Card
		direction      Direction
		answer         string
		wantGrade      Grade
		wantCorrection string
	}{
		{name: "Polish case and spaces at the ends", card: slowo, direction: RuPl, answer: " słowo ", wantGrade: Correct, wantCorrection: "Słowo"},
		{name: "Russian case and spaces at the ends", card: slowo, direction: PlRu, answer: "  СЛОВО  ", wantGrade: Correct, wantCorrection: "Слово"},
		{name: "a decomposed Polish answer", card: zolw, direction: RuPl, answer: "z\u0307o\u0301łw", wantGrade: Correct, wantCorrection: "Żółw"},
		{name: "a run of white space in an expression", card: Card{Pl: "Zielona herbata", Ru: "Зелёный чай"}, direction: RuPl, answer: " ZIELONA \n HERBATA ", wantGrade: Correct, wantCorrection: "Zielona herbata"},
		{name: "a Polish alternative", card: slowo, direction: RuPl, answer: " WYRAZ ", wantGrade: Correct, wantCorrection: "Słowo"},
		{name: "a Russian alternative", card: slowo, direction: PlRu, answer: "словечко", wantGrade: Correct, wantCorrection: "Слово"},
		{name: "е for ё", card: Card{Pl: "Zielony", Ru: "Зелёный"}, direction: PlRu, answer: "зеленый", wantGrade: Correct, wantCorrection: "Зелёный"},
		{name: "a missing ł", card: slowo, direction: RuPl, answer: "slowo", wantGrade: Almost, wantCorrection: "Słowo"},
		{name: "missing ż, ó and ł", card: zolw, direction: RuPl, answer: "zolw", wantGrade: Almost, wantCorrection: "Żółw"},
		{name: "a missing ó in an alternative gives that alternative", card: Card{Pl: "Słowo", Ru: "Слово", AlternativesPl: []string{"Słówko"}}, direction: RuPl, answer: "słowko", wantGrade: Almost, wantCorrection: "Słówko"},
		{name: "the first term that matches is the correction", card: Card{Pl: "Słowo", Ru: "Слово", AlternativesPl: []string{"Słówo"}}, direction: RuPl, answer: "slowo", wantGrade: Almost, wantCorrection: "Słowo"},
		{name: "an exact alternative wins over an almost term", card: Card{Pl: "Słowo", Ru: "Слово", AlternativesPl: []string{"slowo"}}, direction: RuPl, answer: "slowo", wantGrade: Correct, wantCorrection: "Słowo"},
		{name: "an extra Polish mark is almost", card: Card{Pl: "Dom", Ru: "Дом"}, direction: RuPl, answer: "dóm", wantGrade: Almost, wantCorrection: "Dom"},
		{name: "a Russian term with Polish letters has no almost grade", card: Card{Pl: "Żabka", Ru: "Żabka"}, direction: PlRu, answer: "zabka", wantGrade: Again, wantCorrection: "Żabka"},
		{name: "и for й is again", card: Card{Pl: "Mój", Ru: "Мой"}, direction: PlRu, answer: "мои", wantGrade: Again, wantCorrection: "Мой"},
		{name: "a Russian answer in the Polish direction", card: slowo, direction: RuPl, answer: "Слово", wantGrade: Again, wantCorrection: "Słowo"},
		{name: "a Polish alternative in the Russian direction", card: slowo, direction: PlRu, answer: "Wyraz", wantGrade: Again, wantCorrection: "Слово"},
		{name: "an empty answer", card: slowo, direction: RuPl, answer: "", wantGrade: Again, wantCorrection: "Słowo"},
		{name: "only white space", card: slowo, direction: PlRu, answer: " \t\n ", wantGrade: Again, wantCorrection: "Слово"},
		{name: "punctuation is a difference", card: slowo, direction: RuPl, answer: "słowo.", wantGrade: Again, wantCorrection: "Słowo"},
		{name: "a mark that is not Polish is again", card: Card{Pl: "Exposé", Ru: "Экспозе"}, direction: RuPl, answer: "expose", wantGrade: Again, wantCorrection: "Exposé"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := grade(tt.card, tt.direction, tt.answer)
			if got.Grade != tt.wantGrade || got.Correction != tt.wantCorrection {
				t.Errorf("grade(%q) = %+v, want {Grade:%s Correction:%s}", tt.answer, got, tt.wantGrade, tt.wantCorrection)
			}
		})
	}
}

// The attempt IDs of the review tests.
const (
	attemptA1 = "00000000-0000-0000-0000-0000000000a1"
	attemptA2 = "00000000-0000-0000-0000-0000000000a2"
)

// reviewBody is the JSON body of a review with the five members.
func reviewBody(attemptID, cardID string, progressVersion int, direction Direction, answer string) string {
	// A ReviewCreate always marshals.
	b, _ := json.Marshal(ReviewCreate{AttemptId: attemptID, CardId: cardID, ProgressVersion: progressVersion, Direction: direction, Answer: answer})
	return string(b)
}

// reviewRow is a row of the table reviews.
type reviewRow struct {
	userID, attemptID, cardID string
	grade                     Grade
}

// reviewRows reads all reviews rows.
func reviewRows(t *testing.T, pool *pgxpool.Pool) map[reviewRow]bool {
	t.Helper()
	rows, err := pool.Query(context.Background(), "SELECT user_id, attempt_id::text, card_id::text, grade FROM reviews")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	m := map[reviewRow]bool{}
	for rows.Next() {
		var r reviewRow
		if err := rows.Scan(&r.userID, &r.attemptID, &r.cardID, &r.grade); err != nil {
			t.Fatal(err)
		}
		m[r] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return m
}

// progressKey names the progress row of a learner for a card.
type progressKey struct{ userID, cardID string }

// progressRows reads all progress rows.
func progressRows(t *testing.T, pool *pgxpool.Pool) map[progressKey]progress {
	t.Helper()
	rows, err := pool.Query(context.Background(), "SELECT user_id, card_id, correct, incorrect, last_correct_at, last_incorrect_at, version FROM card_progress")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	m := map[progressKey]progress{}
	for rows.Next() {
		var k progressKey
		var p progress
		if err := rows.Scan(&k.userID, &k.cardID, &p.correct, &p.incorrect, &p.lastCorrectAt, &p.lastIncorrectAt, &p.version); err != nil {
			t.Fatal(err)
		}
		m[k] = p
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return m
}

// equalProgress compares the times by instant.
func equalProgress(a, b progress) bool {
	equalTime := func(x, y *time.Time) bool { return x == nil && y == nil || x != nil && y != nil && x.Equal(*y) }
	return a.correct == b.correct && a.incorrect == b.incorrect && a.version == b.version &&
		equalTime(a.lastCorrectAt, b.lastCorrectAt) && equalTime(a.lastIncorrectAt, b.lastIncorrectAt)
}

// wantProgress is the progress row of the caller for K1 after the request. A time is "new" for a time of the
// request, "kept" for the time that the setup stored, and empty for NULL. The zero value is no row, because a row
// that progressRows does not have reads as the zero progress.
type wantProgress struct {
	correct, incorrect             int
	lastCorrectAt, lastIncorrectAt string
	version                        int
}

// checkProgress checks the row got against want. setup is the row before the request. A new time is not more than
// 1 second before start and not more than 1 second after end.
func checkProgress(t *testing.T, got, setup progress, want wantProgress, start, end time.Time) {
	t.Helper()
	if got.correct != want.correct || got.incorrect != want.incorrect || got.version != want.version {
		t.Errorf("progress = (%d, %d, version %d), want (%d, %d, version %d)", got.correct, got.incorrect, got.version, want.correct, want.incorrect, want.version)
	}
	checkTime := func(column string, got, setup *time.Time, want string) {
		var ok bool
		switch want {
		case "":
			ok = got == nil
		case "new":
			ok = got != nil && !got.Before(start.Add(-time.Second)) && !got.After(end.Add(time.Second))
		case "kept":
			ok = got != nil && setup != nil && got.Equal(*setup)
		}
		if !ok {
			t.Errorf("%s = %v, want %q (setup %v, request from %v to %v)", column, got, want, setup, start, end)
		}
	}
	checkTime("last_correct_at", got.lastCorrectAt, setup.lastCorrectAt, want.lastCorrectAt)
	checkTime("last_incorrect_at", got.lastIncorrectAt, setup.lastIncorrectAt, want.lastIncorrectAt)
}

func TestCreateReview(t *testing.T) {
	// addProgress takes the rows (learner, card, correct, incorrect, last correct, last incorrect, version).
	const addProgress = "INSERT INTO card_progress (user_id, card_id, correct, incorrect, last_correct_at, last_incorrect_at, version) VALUES "
	// addReview takes the rows (learner, attempt, card, grade).
	const addReview = "INSERT INTO reviews (user_id, attempt_id, card_id, grade) VALUES "
	const (
		attemptRule   = `{"message":"attemptId must be a UUID"}`
		versionRule   = `{"message":"progressVersion must be 0 or more"}`
		directionRule = `{"message":"direction must be ru-pl or pl-ru"}`
		answerRule    = `{"message":"answer must have at most 500 characters"}`
		notCurrent    = `{"message":"progressVersion is not current"}`
		otherCard     = `{"message":"attemptId is used for another card"}`
	)
	tests := []struct {
		name       string
		setup      string // the SQL text that runs after groupFixture and cardFixture
		sub        string
		body       string
		wantStatus int
		wantBody   string // the body for a status that is not 200 or 201
		wantReview Review // the body for 200 and 201
		wantRow    wantProgress
	}{
		{name: "the first correct answer makes the progress row", sub: subOla, body: reviewBody(attemptA1, cardK1.Id, 0, RuPl, "dom"),
			wantStatus: 201, wantReview: Review{Grade: Correct, Correction: "dom"}, wantRow: wantProgress{1, 0, "new", "", 1}},
		{name: "the first miss makes the progress row", sub: subOla, body: reviewBody(attemptA1, cardK1.Id, 0, RuPl, "kot"),
			wantStatus: 201, wantReview: Review{Grade: Again, Correction: "dom"}, wantRow: wantProgress{0, 1, "", "new", 1}},
		{name: "an almost answer counts as a miss", sub: subOla, body: reviewBody(attemptA1, cardK1.Id, 0, RuPl, "dóm"),
			wantStatus: 201, wantReview: Review{Grade: Almost, Correction: "dom"}, wantRow: wantProgress{0, 1, "", "new", 1}},
		{name: "an empty answer counts as a miss", sub: subOla, body: reviewBody(attemptA1, cardK1.Id, 0, RuPl, ""),
			wantStatus: 201, wantReview: Review{Grade: Again, Correction: "dom"}, wantRow: wantProgress{0, 1, "", "new", 1}},
		{name: "a correct answer in the Russian direction", sub: subOla, body: reviewBody(attemptA1, cardK1.Id, 0, PlRu, "ДОМ"),
			wantStatus: 201, wantReview: Review{Grade: Correct, Correction: "дом"}, wantRow: wantProgress{1, 0, "new", "", 1}},
		{name: "a correct answer after a miss keeps the time of the miss", sub: subOla, body: reviewBody(attemptA1, cardK1.Id, 1, RuPl, "dom"),
			setup:      addProgress + "('sub-ola', '00000000-0000-0000-0000-000000000001', 0, 1, NULL, now() - interval '1 hour', 1)",
			wantStatus: 201, wantReview: Review{Grade: Correct, Correction: "dom"}, wantRow: wantProgress{1, 1, "new", "kept", 2}},
		// After 2 correct answers the card waits 3 days, so it is not due.
		{name: "a miss on a card that is not due keeps the time of the correct answer", sub: subOla, body: reviewBody(attemptA1, cardK1.Id, 2, RuPl, "kot"),
			setup:      addProgress + "('sub-ola', '00000000-0000-0000-0000-000000000001', 2, 0, now() - interval '2 days', NULL, 2)",
			wantStatus: 201, wantReview: Review{Grade: Again, Correction: "dom"}, wantRow: wantProgress{2, 1, "kept", "new", 3}},
		{name: "the progress of another learner stays", sub: subOla, body: reviewBody(attemptA1, cardK1.Id, 0, RuPl, "dom"),
			setup:      addProgress + "('sub-adam', '00000000-0000-0000-0000-000000000001', 3, 1, now() - interval '1 day', now() - interval '2 days', 4)",
			wantStatus: 201, wantReview: Review{Grade: Correct, Correction: "dom"}, wantRow: wantProgress{1, 0, "new", "", 1}},
		{name: "an answer of 500 characters outside the BMP passes", sub: subOla, body: reviewBody(attemptA1, cardK1.Id, 0, RuPl, strings.Repeat("\U0001F600", 500)),
			wantStatus: 201, wantReview: Review{Grade: Again, Correction: "dom"}, wantRow: wantProgress{0, 1, "", "new", 1}},
		{name: "a body that is not JSON", sub: subOla, body: `{`, wantStatus: 400, wantBody: notJSON},
		{name: "a negative progress version", sub: subOla, body: reviewBody(attemptA1, cardK1.Id, -1, RuPl, "dom"), wantStatus: 400, wantBody: versionRule},
		{name: "a direction outside the two values", sub: subOla, body: reviewBody(attemptA1, cardK1.Id, 0, "en-pl", "dom"), wantStatus: 400, wantBody: directionRule},
		{name: "a body without a direction", sub: subOla, body: `{"attemptId":"00000000-0000-0000-0000-0000000000a1","cardId":"00000000-0000-0000-0000-000000000001","progressVersion":0,"answer":"dom"}`,
			wantStatus: 400, wantBody: directionRule},
		{name: "an answer of 501 characters", sub: subOla, body: reviewBody(attemptA1, cardK1.Id, 0, RuPl, strings.Repeat("a", 501)), wantStatus: 400, wantBody: answerRule},
		{name: "the first broken rule gives the text", sub: subOla, body: reviewBody(attemptA1, cardK1.Id, -1, "en-pl", "dom"), wantStatus: 400, wantBody: versionRule},
		{name: "a body without an attempt ID", sub: subOla, body: `{"cardId":"00000000-0000-0000-0000-000000000001","progressVersion":0,"direction":"ru-pl","answer":"dom"}`,
			wantStatus: 400, wantBody: attemptRule},
		{name: "the attempt ID is the first rule", sub: subOla, body: reviewBody("abc", cardK1.Id, -1, RuPl, "dom"), wantStatus: 400, wantBody: attemptRule},
		{name: "a card ID that is not a UUID", sub: subOla, body: reviewBody(attemptA1, "abc", 0, RuPl, "dom"), wantStatus: 404, wantBody: cardNotFound},
		{name: "a card that does not exist", sub: subOla, body: reviewBody(attemptA1, "00000000-0000-0000-0000-000000000009", 0, RuPl, "dom"), wantStatus: 404, wantBody: cardNotFound},
		{name: "an archived card", sub: subOla, body: reviewBody(attemptA1, cardK2.Id, 0, RuPl, "dom"), wantStatus: 404, wantBody: cardNotFound},
		{name: "a card of another group", sub: subOla, body: reviewBody(attemptA1, cardK3.Id, 0, RuPl, "dom"), wantStatus: 404, wantBody: cardNotFound},
		{name: "a grammar card", sub: subOla, body: reviewBody(attemptA1, cardK1.Id, 0, RuPl, "dom"),
			setup: "UPDATE cards SET kind = 'grammar' WHERE id = '00000000-0000-0000-0000-000000000001'", wantStatus: 404, wantBody: cardNotFound},
		{name: "the version 0 for a card with a progress row", sub: subOla, body: reviewBody(attemptA1, cardK1.Id, 0, RuPl, "dom"),
			setup:      addProgress + "('sub-ola', '00000000-0000-0000-0000-000000000001', 1, 0, now() - interval '2 days', NULL, 1)",
			wantStatus: 409, wantBody: notCurrent, wantRow: wantProgress{1, 0, "kept", "", 1}},
		{name: "an old version", sub: subOla, body: reviewBody(attemptA1, cardK1.Id, 2, RuPl, "dom"),
			setup:      addProgress + "('sub-ola', '00000000-0000-0000-0000-000000000001', 2, 1, now() - interval '2 days', now() - interval '3 days', 3)",
			wantStatus: 409, wantBody: notCurrent, wantRow: wantProgress{2, 1, "kept", "kept", 3}},
		{name: "a version for a card without a progress row", sub: subOla, body: reviewBody(attemptA1, cardK1.Id, 1, RuPl, "dom"), wantStatus: 409, wantBody: notCurrent},
		{name: "a repeat gives the stored grade and counts nothing", sub: subOla, body: reviewBody(attemptA1, cardK1.Id, 0, RuPl, "dom"),
			setup: addProgress + "('sub-ola', '00000000-0000-0000-0000-000000000001', 1, 0, now() - interval '1 hour', NULL, 1);" +
				addReview + "('sub-ola', '00000000-0000-0000-0000-0000000000a1', '00000000-0000-0000-0000-000000000001', 'correct')",
			wantStatus: 200, wantReview: Review{Grade: Correct, Correction: "dom"}, wantRow: wantProgress{1, 0, "kept", "", 1}},
		{name: "a repeat with another answer gives the stored grade", sub: subOla, body: reviewBody(attemptA1, cardK1.Id, 0, RuPl, "dom"),
			setup: addProgress + "('sub-ola', '00000000-0000-0000-0000-000000000001', 0, 1, NULL, now() - interval '1 hour', 1);" +
				addReview + "('sub-ola', '00000000-0000-0000-0000-0000000000a1', '00000000-0000-0000-0000-000000000001', 'again')",
			wantStatus: 200, wantReview: Review{Grade: Again, Correction: "dom"}, wantRow: wantProgress{0, 1, "", "kept", 1}},
		{name: "a repeat of an almost grade gives the term that the answer matches", sub: subOla, body: reviewBody(attemptA1, cardK1.Id, 0, RuPl, "dómek"),
			setup: addProgress + "('sub-ola', '00000000-0000-0000-0000-000000000001', 0, 1, NULL, now() - interval '1 hour', 1);" +
				addReview + "('sub-ola', '00000000-0000-0000-0000-0000000000a1', '00000000-0000-0000-0000-000000000001', 'almost')",
			wantStatus: 200, wantReview: Review{Grade: Almost, Correction: "domek"}, wantRow: wantProgress{0, 1, "", "kept", 1}},
		{name: "a repeat of an almost grade whose answer matches no term gives the main term", sub: subOla, body: reviewBody(attemptA1, cardK1.Id, 0, RuPl, "kot"),
			setup: addProgress + "('sub-ola', '00000000-0000-0000-0000-000000000001', 0, 1, NULL, now() - interval '1 hour', 1);" +
				addReview + "('sub-ola', '00000000-0000-0000-0000-0000000000a1', '00000000-0000-0000-0000-000000000001', 'almost')",
			wantStatus: 200, wantReview: Review{Grade: Almost, Correction: "dom"}, wantRow: wantProgress{0, 1, "", "kept", 1}},
		{name: "a repeat in the Russian direction gives the Russian term", sub: subOla, body: reviewBody(attemptA1, cardK1.Id, 0, PlRu, "дом"),
			setup: addProgress + "('sub-ola', '00000000-0000-0000-0000-000000000001', 1, 0, now() - interval '1 hour', NULL, 1);" +
				addReview + "('sub-ola', '00000000-0000-0000-0000-0000000000a1', '00000000-0000-0000-0000-000000000001', 'correct')",
			wantStatus: 200, wantReview: Review{Grade: Correct, Correction: "дом"}, wantRow: wantProgress{1, 0, "kept", "", 1}},
		{name: "a repeat with the attempt ID in upper case", sub: subOla, body: reviewBody("00000000-0000-0000-0000-0000000000A1", cardK1.Id, 0, RuPl, "dom"),
			setup: addProgress + "('sub-ola', '00000000-0000-0000-0000-000000000001', 1, 0, now() - interval '1 hour', NULL, 1);" +
				addReview + "('sub-ola', '00000000-0000-0000-0000-0000000000a1', '00000000-0000-0000-0000-000000000001', 'correct')",
			wantStatus: 200, wantReview: Review{Grade: Correct, Correction: "dom"}, wantRow: wantProgress{1, 0, "kept", "", 1}},
		{name: "a repeat after the archive of the card", sub: subOla, body: reviewBody(attemptA1, cardK2.Id, 0, RuPl, "stary"),
			setup: addProgress + "('sub-ola', '00000000-0000-0000-0000-000000000002', 1, 0, now() - interval '1 hour', NULL, 1);" +
				addReview + "('sub-ola', '00000000-0000-0000-0000-0000000000a1', '00000000-0000-0000-0000-000000000002', 'correct')",
			wantStatus: 200, wantReview: Review{Grade: Correct, Correction: "stary"}},
		{name: "the attempt ID of another card", sub: subOla, body: reviewBody(attemptA1, cardK1.Id, 0, RuPl, "dom"),
			setup:      addReview + "('sub-ola', '00000000-0000-0000-0000-0000000000a1', '00000000-0000-0000-0000-000000000002', 'correct')",
			wantStatus: 409, wantBody: otherCard},
		{name: "the attempt ID of another learner is free", sub: subOla, body: reviewBody(attemptA1, cardK1.Id, 0, RuPl, "dom"),
			setup:      addReview + "('sub-adam', '00000000-0000-0000-0000-0000000000a1', '00000000-0000-0000-0000-000000000001', 'again')",
			wantStatus: 201, wantReview: Review{Grade: Correct, Correction: "dom"}, wantRow: wantProgress{1, 0, "new", "", 1}},
		{name: "a repeat for a card of another group", sub: subOla, body: reviewBody(attemptA1, cardK3.Id, 0, RuPl, "dom"),
			setup:      addReview + "('sub-ola', '00000000-0000-0000-0000-0000000000a1', '00000000-0000-0000-0000-000000000003', 'correct')",
			wantStatus: 404, wantBody: cardNotFound},
		{name: "a member of another group is refused", sub: subEwa, body: reviewBody(attemptA1, cardK1.Id, 0, RuPl, "dom"), wantStatus: 403, wantBody: notMember},
		{name: "a user without a group is refused", sub: subJan, body: reviewBody(attemptA1, cardK1.Id, 0, RuPl, "dom"), wantStatus: 403, wantBody: notMember},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			pool, srv, rodzina, _ := groupFixture(t)
			if _, err := pool.Exec(ctx, cardFixture+";"+tt.setup); err != nil {
				t.Fatal(err)
			}
			key := progressKey{tt.sub, cardK1.Id}
			before := progressRows(t, pool)
			wantReviews := reviewRows(t, pool)

			start := time.Now()
			resp, err := srv.Handle(ctx, groupRequest("POST /v1/groups/{groupId}/reviews", tt.sub, tt.body, map[string]string{"groupId": rodzina}))
			end := time.Now()
			if err != nil {
				t.Fatalf("Handle returned error %v", err)
			}
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d (body %s)", resp.StatusCode, tt.wantStatus, resp.Body)
			}
			if tt.wantStatus == http.StatusOK || tt.wantStatus == http.StatusCreated {
				var review Review
				if err := json.Unmarshal([]byte(resp.Body), &review); err != nil {
					t.Fatalf("body %s: %v", resp.Body, err)
				}
				if review != tt.wantReview {
					t.Errorf("review = %+v, want %+v", review, tt.wantReview)
				}
			} else if resp.Body != tt.wantBody {
				t.Errorf("body = %s, want %s", resp.Body, tt.wantBody)
			}
			after := progressRows(t, pool)
			checkProgress(t, after[key], before[key], tt.wantRow, start, end)
			delete(after, key)
			delete(before, key)
			if !maps.EqualFunc(after, before, equalProgress) {
				t.Errorf("the other progress rows = %v, want %v", after, before)
			}
			// Only a new review stores a row.
			if tt.wantStatus == http.StatusCreated {
				var sent ReviewCreate
				if err := json.Unmarshal([]byte(tt.body), &sent); err != nil {
					t.Fatal(err)
				}
				wantReviews[reviewRow{tt.sub, sent.AttemptId, sent.CardId, tt.wantReview.Grade}] = true
			}
			if got := reviewRows(t, pool); !maps.Equal(got, wantReviews) {
				t.Errorf("reviews = %v, want %v", got, wantReviews)
			}
		})
	}
}

// TestCreateReviewAtTheSameInstant sends two answers of Ola to K1 with one progressVersion, and makes both of them
// read the progress version before the other one writes the row. For a new card, the held transaction locks the card
// row: the insert of the progress row checks its foreign key to the card row, so it waits for that lock.
func TestCreateReviewAtTheSameInstant(t *testing.T) {
	tests := []struct {
		name                        string
		setup                       string // the SQL text that runs after groupFixture and cardFixture
		lock                        string // the SQL text that locks a row in the held transaction
		progressVersion             int
		secondAttempt, secondAnswer string
		wantFirst, wantSecond       int
		wantRow                     wantProgress
		wantReviews                 int // the reviews rows of Ola after the two requests
	}{
		{name: "a card with a progress row",
			setup: "INSERT INTO card_progress (user_id, card_id, correct, incorrect, last_correct_at, last_incorrect_at, version) " +
				"VALUES ('sub-ola', '00000000-0000-0000-0000-000000000001', 1, 0, now() - interval '2 days', NULL, 1)",
			lock:            "SELECT version FROM card_progress WHERE card_id = '00000000-0000-0000-0000-000000000001' AND user_id = 'sub-ola' FOR UPDATE",
			progressVersion: 1, secondAttempt: attemptA2, secondAnswer: "kot", wantFirst: 201, wantSecond: 409, wantRow: wantProgress{2, 0, "new", "", 2}, wantReviews: 1},
		{name: "a new card",
			lock:            "SELECT id FROM cards WHERE id = '00000000-0000-0000-0000-000000000001' FOR UPDATE",
			progressVersion: 0, secondAttempt: attemptA2, secondAnswer: "kot", wantFirst: 201, wantSecond: 409, wantRow: wantProgress{1, 0, "new", "", 1}, wantReviews: 1},
		{name: "one attempt sent two times, a card with a progress row",
			setup: "INSERT INTO card_progress (user_id, card_id, correct, incorrect, last_correct_at, last_incorrect_at, version) " +
				"VALUES ('sub-ola', '00000000-0000-0000-0000-000000000001', 1, 0, now() - interval '2 days', NULL, 1)",
			lock:            "SELECT version FROM card_progress WHERE card_id = '00000000-0000-0000-0000-000000000001' AND user_id = 'sub-ola' FOR UPDATE",
			progressVersion: 1, secondAttempt: attemptA1, secondAnswer: "dom", wantFirst: 201, wantSecond: 200, wantRow: wantProgress{2, 0, "new", "", 2}, wantReviews: 1},
		{name: "one attempt sent two times, a new card",
			lock:            "SELECT id FROM cards WHERE id = '00000000-0000-0000-0000-000000000001' FOR UPDATE",
			progressVersion: 0, secondAttempt: attemptA1, secondAnswer: "dom", wantFirst: 201, wantSecond: 200, wantRow: wantProgress{1, 0, "new", "", 1}, wantReviews: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			pool, srv, rodzina, _ := groupFixture(t)
			if _, err := pool.Exec(ctx, cardFixture+";"+tt.setup); err != nil {
				t.Fatal(err)
			}
			key := progressKey{subOla, cardK1.Id}
			before := progressRows(t, pool)
			send := func(attemptID, answer string) <-chan events.APIGatewayV2HTTPResponse {
				req := groupRequest("POST /v1/groups/{groupId}/reviews", subOla, reviewBody(attemptID, cardK1.Id, tt.progressVersion, RuPl, answer), map[string]string{"groupId": rodzina})
				out := make(chan events.APIGatewayV2HTTPResponse, 1)
				go func() {
					resp, _ := srv.Handle(ctx, req)
					out <- resp
				}()
				return out
			}

			// The held lock stops the write of the first request, and the write of the second request waits for the
			// first request. So each request reads the version before a write. The rollback frees the first request.
			held, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = held.Rollback(ctx) }()
			if _, err := held.Exec(ctx, tt.lock); err != nil {
				t.Fatal(err)
			}
			start := time.Now()
			first := send(attemptA1, "dom")
			waitForLockWaits(t, pool, 1)
			second := send(tt.secondAttempt, tt.secondAnswer)
			waitForLockWaits(t, pool, 2)
			if err := held.Rollback(ctx); err != nil {
				t.Fatal(err)
			}

			if got := (<-first).StatusCode; got != tt.wantFirst {
				t.Errorf("status of the first request = %d, want %d", got, tt.wantFirst)
			}
			resp := <-second
			if resp.StatusCode != tt.wantSecond {
				t.Errorf("status of the second request = %d, want %d", resp.StatusCode, tt.wantSecond)
			}
			if tt.wantSecond == http.StatusOK {
				var review Review
				if err := json.Unmarshal([]byte(resp.Body), &review); err != nil {
					t.Fatalf("body %s: %v", resp.Body, err)
				}
				if want := (Review{Grade: Correct, Correction: "dom"}); review != want {
					t.Errorf("review of the second request = %+v, want %+v", review, want)
				}
			}
			end := time.Now()
			checkProgress(t, progressRows(t, pool)[key], before[key], tt.wantRow, start, end)
			var reviews int
			if err := pool.QueryRow(ctx, "SELECT count(*) FROM reviews WHERE user_id = 'sub-ola'").Scan(&reviews); err != nil {
				t.Fatal(err)
			}
			if reviews != tt.wantReviews {
				t.Errorf("reviews of Ola = %d, want %d", reviews, tt.wantReviews)
			}
		})
	}
}
