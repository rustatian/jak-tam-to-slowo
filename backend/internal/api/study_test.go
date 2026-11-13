package api

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"
)

func TestDueAt(t *testing.T) {
	t0 := time.Date(2026, 10, 21, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name                           string
		correct, incorrect             int
		lastCorrectAt, lastIncorrectAt *time.Time
		want                           time.Time
	}{
		{name: "a miss", correct: 0, incorrect: 1, lastIncorrectAt: new(t0.Add(-time.Second)), want: t0.Add(-time.Second + 10*time.Minute)},
		{name: "a miss after correct answers", correct: 5, incorrect: 1, lastCorrectAt: new(t0.Add(-24 * time.Hour)), lastIncorrectAt: new(t0.Add(-time.Second)), want: t0.Add(-time.Second + 10*time.Minute)},
		{name: "equal times are a miss", correct: 1, incorrect: 1, lastCorrectAt: new(t0.Add(-time.Second)), lastIncorrectAt: new(t0.Add(-time.Second)), want: t0.Add(-time.Second + 10*time.Minute)},
		{name: "a correct answer after many misses", correct: 1, incorrect: 10, lastCorrectAt: new(t0.Add(-time.Second)), lastIncorrectAt: new(t0.Add(-time.Hour)), want: t0.Add(-time.Second + 4*time.Hour)},
		{name: "a correct answer at score 0", correct: 2, incorrect: 1, lastCorrectAt: new(t0.Add(-time.Second)), lastIncorrectAt: new(t0.Add(-time.Hour)), want: t0.Add(-time.Second + 4*time.Hour)},
		{name: "a correct answer at score 1", correct: 1, incorrect: 0, lastCorrectAt: new(t0.Add(-time.Second)), want: t0.Add(-time.Second + 24*time.Hour)},
		{name: "a correct answer at score 2", correct: 2, incorrect: 0, lastCorrectAt: new(t0.Add(-time.Second)), want: t0.Add(-time.Second + 72*time.Hour)},
		{name: "a correct answer at score 3", correct: 3, incorrect: 0, lastCorrectAt: new(t0.Add(-time.Second)), want: t0.Add(-time.Second + 168*time.Hour)},
		{name: "a correct answer at score 4 after a miss", correct: 6, incorrect: 1, lastCorrectAt: new(t0.Add(-time.Second)), lastIncorrectAt: new(t0.Add(-24 * time.Hour)), want: t0.Add(-time.Second + 336*time.Hour)},
		{name: "a correct answer at score 5", correct: 5, incorrect: 0, lastCorrectAt: new(t0.Add(-time.Second)), want: t0.Add(-time.Second + 720*time.Hour)},
		{name: "a correct answer at score 6", correct: 6, incorrect: 0, lastCorrectAt: new(t0.Add(-time.Second)), want: t0.Add(-time.Second + 1440*time.Hour)},
		{name: "a correct answer above score 6", correct: 9, incorrect: 0, lastCorrectAt: new(t0.Add(-time.Second)), want: t0.Add(-time.Second + 1440*time.Hour)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := progress{correct: tt.correct, incorrect: tt.incorrect, lastCorrectAt: tt.lastCorrectAt, lastIncorrectAt: tt.lastIncorrectAt, version: 1}
			if got := p.dueAt(); !got.Equal(tt.want) {
				t.Errorf("dueAt() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStudyQueue(t *testing.T) {
	t0 := time.Date(2026, 10, 21, 10, 0, 0, 0, time.UTC)
	row := func(id string, createdAt time.Time, p progress) studyRow {
		return studyRow{card: Card{Id: id}, createdAt: createdAt, progress: p}
	}
	// The base cards with no progress. A progress is (correct, incorrect, last correct, last incorrect, version).
	a := row("a", t0.Add(-100*time.Millisecond), progress{})
	b := row("b", t0.Add(-300*time.Millisecond), progress{})
	c := row("c", t0.Add(-200*time.Millisecond), progress{})
	d := row("d", t0.Add(-400*time.Millisecond), progress{})
	waiting := []studyRow{row("a", a.createdAt, progress{1, 0, new(t0.Add(-time.Second)), nil, 3}), b, c, d}
	// a, then the new cards n12 down to n01. n01 is made at t0 - 12 ms, and each next one 1 ms later.
	overLimit := []studyRow{row("a", a.createdAt, progress{1, 0, new(t0), nil, 1})}
	for i := 12; i >= 1; i-- {
		overLimit = append(overLimit, row(fmt.Sprintf("n%02d", i), t0.Add(time.Duration(i-13)*time.Millisecond), progress{}))
	}
	tests := []struct {
		name         string
		rows         []studyRow
		now          time.Time
		wantIDs      []string
		wantVersions []int
		wantNext     *time.Time
	}{
		{
			name:         "new cards are due, the oldest first",
			rows:         []studyRow{a, b, c, d},
			now:          t0,
			wantIDs:      []string{"d", "b", "c", "a"},
			wantVersions: []int{0, 0, 0, 0},
		},
		{
			name:         "1 ms before the due time the card waits",
			rows:         waiting,
			now:          t0.Add(-time.Second + 24*time.Hour - time.Millisecond),
			wantIDs:      []string{"d", "b", "c"},
			wantVersions: []int{0, 0, 0},
			wantNext:     new(t0.Add(-time.Second + 24*time.Hour)),
		},
		{
			name:         "at the due time the card is in the queue",
			rows:         waiting,
			now:          t0.Add(-time.Second + 24*time.Hour),
			wantIDs:      []string{"d", "b", "c", "a"},
			wantVersions: []int{0, 0, 0, 3},
		},
		{
			name: "more misses for each correct answer come first",
			rows: []studyRow{
				row("a", a.createdAt, progress{1, 3, new(t0.Add(-48 * time.Hour)), new(t0.Add(-time.Hour)), 3}),
				row("b", b.createdAt, progress{2, 0, new(t0.Add(-96 * time.Hour)), nil, 2}),
				c,
				d,
			},
			now:          t0,
			wantIDs:      []string{"a", "d", "c", "b"},
			wantVersions: []int{3, 0, 0, 2},
		},
		{
			name: "equal weights: the earlier time, then the ID",
			rows: []studyRow{
				d,
				row("c", t0.Add(-400*time.Millisecond), progress{}),
				row("b", t0.Add(-400*time.Millisecond), progress{}),
				row("a", t0.Add(-100*time.Millisecond), progress{1, 1, new(t0.Add(-24 * time.Hour)), new(t0.Add(-time.Hour)), 2}),
			},
			now:          t0,
			wantIDs:      []string{"a", "b", "c", "d"},
			wantVersions: []int{2, 0, 0, 0},
		},
		{
			name:         "the queue has the first 10 cards",
			rows:         overLimit,
			now:          t0,
			wantIDs:      []string{"n01", "n02", "n03", "n04", "n05", "n06", "n07", "n08", "n09", "n10"},
			wantVersions: make([]int, 10),
			wantNext:     new(t0.Add(24 * time.Hour)),
		},
		{
			name: "nextDueAt is the earliest due time after now",
			rows: []studyRow{
				row("a", a.createdAt, progress{0, 1, nil, new(t0.Add(-time.Minute)), 1}),
				row("b", b.createdAt, progress{1, 0, new(t0.Add(-24*time.Hour + 5*time.Minute)), nil, 1}),
				row("c", c.createdAt, progress{1, 0, new(t0.Add(-time.Hour)), nil, 1}),
				d,
			},
			now:          t0,
			wantIDs:      []string{"d"},
			wantVersions: []int{0},
			wantNext:     new(t0.Add(5 * time.Minute)),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := studyQueue(tt.rows, tt.now)
			var ids []string
			var versions []int
			for _, sc := range got.Cards {
				ids = append(ids, sc.Card.Id)
				versions = append(versions, sc.ProgressVersion)
			}
			if !slices.Equal(ids, tt.wantIDs) || !slices.Equal(versions, tt.wantVersions) {
				t.Errorf("studyQueue() cards = %v, versions %v, want %v, versions %v", ids, versions, tt.wantIDs, tt.wantVersions)
			}
			if (got.NextDueAt == nil) != (tt.wantNext == nil) || got.NextDueAt != nil && !got.NextDueAt.Equal(*tt.wantNext) {
				t.Errorf("studyQueue() NextDueAt = %v, want %v", got.NextDueAt, tt.wantNext)
			}
		})
	}
}

func TestGetStudyQueue(t *testing.T) {
	// addK4 adds the new card K4 to Rodzina, made 1 hour before the other cards of the setup.
	const addK4 = `INSERT INTO cards (group_id, id, pl, ru, example_pl, example_ru, meaning_ru, created_at) VALUES
		((SELECT id FROM groups WHERE name = 'Rodzina'), '00000000-0000-0000-0000-000000000004', 'kot', 'кот', 'To jest kot.', 'Это кот.', 'животное', now() - interval '1 hour')`
	// addProgress takes the rows (learner, card, correct, incorrect, last correct, last incorrect, version).
	const addProgress = "INSERT INTO card_progress (user_id, card_id, correct, incorrect, last_correct_at, last_incorrect_at, version) VALUES "
	cardK4 := Card{Id: "00000000-0000-0000-0000-000000000004", Kind: Translation, Pl: "kot", Ru: "кот", ExamplePl: "To jest kot.", ExampleRu: "Это кот.",
		MeaningRu: "животное", AlternativesPl: []string{}, AlternativesRu: []string{}}
	tests := []struct {
		name         string
		setup        string // the SQL text that runs after groupFixture
		sub          string
		wantStatus   int
		wantBody     string // empty when the answer is checked by want, wantVersions and the next fields
		want         []Card // the full cards of the queue in order
		wantVersions []int
		nextFrom     string        // the column of the progress row of Ola for K1 that nextDueAt counts from; empty for no nextDueAt
		nextWait     time.Duration // nextDueAt minus the time in nextFrom
	}{
		{name: "a group without cards has an empty queue", sub: subOla, wantStatus: 200, wantBody: `{"cards":[]}`},
		{name: "new cards have the version 0, the oldest first", setup: cardFixture + "; " + addK4, sub: subOla, wantStatus: 200,
			want: []Card{cardK4, cardK1}, wantVersions: []int{0, 0}},
		{name: "a card that waits after a correct answer is not in the queue", sub: subOla, wantStatus: 200,
			setup:    cardFixture + "; " + addProgress + "('sub-ola', '00000000-0000-0000-0000-000000000001', 1, 0, now() - interval '1 hour', NULL, 1)",
			nextFrom: "last_correct_at", nextWait: 24 * time.Hour},
		{name: "a card that waits after a miss is not in the queue", sub: subOla, wantStatus: 200,
			setup:    cardFixture + "; " + addProgress + "('sub-ola', '00000000-0000-0000-0000-000000000001', 0, 1, NULL, now() - interval '1 minute', 1)",
			nextFrom: "last_incorrect_at", nextWait: 10 * time.Minute},
		{name: "a due card has its progress version", sub: subOla, wantStatus: 200,
			setup: cardFixture + "; " + addProgress + "('sub-ola', '00000000-0000-0000-0000-000000000001', 1, 0, now() - interval '2 days', NULL, 3)",
			want:  []Card{cardK1}, wantVersions: []int{3}},
		{name: "the card with more misses for each correct answer comes first", sub: subOla, wantStatus: 200,
			setup: cardFixture + "; " + addK4 + "; " + addProgress +
				"('sub-ola', '00000000-0000-0000-0000-000000000001', 1, 3, now() - interval '2 days', now() - interval '1 hour', 4), " +
				"('sub-ola', '00000000-0000-0000-0000-000000000004', 2, 0, now() - interval '4 days', NULL, 2)",
			want: []Card{cardK1, cardK4}, wantVersions: []int{4, 2}},
		{name: "the progress of another learner does not count", sub: subOla, wantStatus: 200,
			setup: cardFixture + "; " + addProgress + "('sub-adam', '00000000-0000-0000-0000-000000000001', 1, 0, now() - interval '1 hour', NULL, 1)",
			want:  []Card{cardK1}, wantVersions: []int{0}},
		{name: "a grammar card is not in the queue", sub: subOla, wantStatus: 200, wantBody: `{"cards":[]}`,
			setup: cardFixture + "; UPDATE cards SET kind = 'grammar' WHERE id = '00000000-0000-0000-0000-000000000001'"},
		{name: "an archived card that waits gives no nextDueAt", sub: subOla, wantStatus: 200,
			setup: cardFixture + "; " + addProgress + "('sub-ola', '00000000-0000-0000-0000-000000000002', 1, 0, now() - interval '1 hour', NULL, 1)",
			want:  []Card{cardK1}, wantVersions: []int{0}},
		{name: "a member of another group is refused", setup: cardFixture, sub: subEwa, wantStatus: 403, wantBody: notMember},
		{name: "a user without a group is refused", setup: cardFixture, sub: subJan, wantStatus: 403, wantBody: notMember},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			pool, srv, rodzina, _ := groupFixture(t)
			if tt.setup != "" {
				if _, err := pool.Exec(ctx, tt.setup); err != nil {
					t.Fatal(err)
				}
			}
			var wantNext *time.Time
			if tt.nextFrom != "" {
				var from time.Time
				if err := pool.QueryRow(ctx, "SELECT "+tt.nextFrom+" FROM card_progress WHERE user_id = $1 AND card_id = $2", subOla, cardK1.Id).Scan(&from); err != nil {
					t.Fatal(err)
				}
				wantNext = new(from.Add(tt.nextWait))
			}
			before := countRows(t, pool, "card_progress")

			resp, err := srv.Handle(ctx, groupRequest("GET /v1/groups/{groupId}/study", tt.sub, "", map[string]string{"groupId": rodzina}))
			if err != nil {
				t.Fatalf("Handle returned error %v", err)
			}
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d (body %s)", resp.StatusCode, tt.wantStatus, resp.Body)
			}
			if n := countRows(t, pool, "card_progress"); n != before {
				t.Errorf("card_progress = %d, want %d", n, before)
			}
			if tt.wantBody != "" {
				if resp.Body != tt.wantBody {
					t.Errorf("body = %s, want %s", resp.Body, tt.wantBody)
				}
				return
			}
			var queue StudyQueue
			if err := json.Unmarshal([]byte(resp.Body), &queue); err != nil {
				t.Fatalf("body %s: %v", resp.Body, err)
			}
			var cards []Card
			var versions []int
			for _, sc := range queue.Cards {
				cards = append(cards, sc.Card)
				versions = append(versions, sc.ProgressVersion)
			}
			// reflect.DeepEqual treats a nil list and an empty list as different.
			if !reflect.DeepEqual(cards, tt.want) || !slices.Equal(versions, tt.wantVersions) {
				t.Errorf("cards = %+v, versions %v, want %+v, versions %v", cards, versions, tt.want, tt.wantVersions)
			}
			var members map[string]any
			if err := json.Unmarshal([]byte(resp.Body), &members); err != nil {
				t.Fatalf("body %s: %v", resp.Body, err)
			}
			if _, has := members["nextDueAt"]; wantNext == nil && has || wantNext != nil && (queue.NextDueAt == nil || !queue.NextDueAt.Equal(*wantNext)) {
				t.Errorf("body %s, want nextDueAt %v", resp.Body, wantNext)
			}
		})
	}
}
