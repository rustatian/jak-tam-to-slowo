package api

import (
	"fmt"
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
