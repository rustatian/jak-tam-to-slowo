package api

import (
	"context"
	"encoding/json/v2"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	plRule        = `{"message":"pl must have 1 to 200 characters"}`
	ruRule        = `{"message":"ru must have 1 to 200 characters"}`
	examplePlRule = `{"message":"examplePl must have 1 to 500 characters"}`
	exampleRuRule = `{"message":"exampleRu must have 1 to 500 characters"}`
	meaningRuRule = `{"message":"meaningRu must have 1 to 200 characters"}`
	altPlRule     = `{"message":"alternativesPl must have at most 8 terms of 1 to 200 characters"}`
	altRuRule     = `{"message":"alternativesRu must have at most 8 terms of 1 to 200 characters"}`
	cardNotFound  = `{"message":"card not found"}`
)

// cardFixture adds the cards K1 and K2 (archived) to Rodzina and K3 to Praca of groupFixture.
const cardFixture = `INSERT INTO cards (group_id, id, pl, ru, example_pl, example_ru, meaning_ru, alternatives_pl, alternatives_ru, archived) VALUES
	((SELECT id FROM groups WHERE name = 'Rodzina'), '00000000-0000-0000-0000-000000000001', 'dom', 'дом', 'To jest dom.', 'Это дом.', 'здание', '["domek"]', '["жилище"]', false),
	((SELECT id FROM groups WHERE name = 'Rodzina'), '00000000-0000-0000-0000-000000000002', 'stary', 'старый', 'To jest stary dom.', 'Это старый дом.', 'не новый', '[]', '[]', true),
	((SELECT id FROM groups WHERE name = 'Praca'), '00000000-0000-0000-0000-000000000003', 'praca', 'работа', 'To jest praca.', 'Это работа.', 'труд', '[]', '[]', false)`

// The cards of cardFixture.
var (
	cardK1 = Card{Id: "00000000-0000-0000-0000-000000000001", Kind: Translation, Pl: "dom", Ru: "дом", ExamplePl: "To jest dom.", ExampleRu: "Это дом.",
		MeaningRu: "здание", AlternativesPl: []string{"domek"}, AlternativesRu: []string{"жилище"}}
	cardK2 = Card{Id: "00000000-0000-0000-0000-000000000002", Kind: Translation, Pl: "stary", Ru: "старый", ExamplePl: "To jest stary dom.", ExampleRu: "Это старый дом.",
		MeaningRu: "не новый", AlternativesPl: []string{}, AlternativesRu: []string{}}
	cardK3 = Card{Id: "00000000-0000-0000-0000-000000000003", Kind: Translation, Pl: "praca", Ru: "работа", ExamplePl: "To jest praca.", ExampleRu: "Это работа.",
		MeaningRu: "труд", AlternativesPl: []string{}, AlternativesRu: []string{}}
)

// cardRow is a row of the table cards.
type cardRow struct {
	card     Card
	group    string
	archived bool
}

// cardRows reads all card rows in ID order. Without ORDER BY, PostgreSQL returns a row that an UPDATE changed after
// the other rows.
// https://www.postgresql.org/docs/16/queries-order.html
func cardRows(t *testing.T, pool *pgxpool.Pool) []cardRow {
	t.Helper()
	rows, err := pool.Query(context.Background(), "SELECT id, kind, pl, ru, example_pl, example_ru, meaning_ru, alternatives_pl, alternatives_ru, group_id, archived FROM cards ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	got, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (cardRow, error) {
		var r cardRow
		c := &r.card
		err := row.Scan(&c.Id, &c.Kind, &c.Pl, &c.Ru, &c.ExamplePl, &c.ExampleRu, &c.MeaningRu, &c.AlternativesPl, &c.AlternativesRu, &r.group, &r.archived)
		return r, err
	})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestCreateCard(t *testing.T) {
	const baseBody = `{"pl":"dom","ru":"дом","examplePl":"To jest dom.","exampleRu":"Это дом.","meaningRu":"здание"}`
	base := Card{Kind: Translation, Pl: "dom", Ru: "дом", ExamplePl: "To jest dom.", ExampleRu: "Это дом.", MeaningRu: "здание", AlternativesPl: []string{}, AlternativesRu: []string{}}
	with := func(change func(*Card)) Card {
		c := base
		change(&c)
		return c
	}
	tests := []struct {
		name       string
		sub        string
		group      string // the group ID of the path; "rodzina" for the ID of Rodzina
		body       string
		wantStatus int
		wantBody   string // empty when the answer is the new card
		want       Card   // the new card without its ID
	}{
		{name: "creates a card with all fields", sub: subOla, group: "rodzina",
			body:       `{"pl":"  Źródło  ","ru":"  дом  ","examplePl":"  To jest dom.  ","exampleRu":"  Это дом.  ","meaningRu":"  здание  ","alternativesPl":[" zdrój "],"alternativesRu":[" родник ","ключ"]}`,
			wantStatus: 201, want: with(func(c *Card) {
				c.Pl = "Źródło"
				c.AlternativesPl = []string{"zdrój"}
				c.AlternativesRu = []string{"родник", "ключ"}
			})},
		{name: "an empty list and a list that is not in the body are stored as []", sub: subAdam, group: "rodzina",
			body:       `{"pl":"dom","ru":"дом","examplePl":"To jest dom.","exampleRu":"Это дом.","meaningRu":"здание","alternativesPl":[]}`,
			wantStatus: 201, want: base},
		{name: "each text at its maximum passes", sub: subOla, group: "rodzina",
			body: `{"pl":"` + strings.Repeat("ż", 200) + `","ru":"` + strings.Repeat("я", 200) + `","examplePl":"` + strings.Repeat("ż", 500) +
				`","exampleRu":"` + strings.Repeat("я", 500) + `","meaningRu":"` + strings.Repeat("я", 200) + `"}`,
			wantStatus: 201, want: with(func(c *Card) {
				c.Pl, c.Ru, c.ExamplePl = strings.Repeat("ż", 200), strings.Repeat("я", 200), strings.Repeat("ż", 500)
				c.ExampleRu, c.MeaningRu = strings.Repeat("я", 500), strings.Repeat("я", 200)
			})},
		{name: "201 letters in pl are rejected", sub: subOla, group: "rodzina",
			body:       `{"pl":"` + strings.Repeat("ż", 201) + `","ru":"дом","examplePl":"To jest dom.","exampleRu":"Это дом.","meaningRu":"здание"}`,
			wantStatus: 400, wantBody: plRule},
		{name: "201 letters in ru are rejected", sub: subOla, group: "rodzina",
			body:       `{"pl":"dom","ru":"` + strings.Repeat("я", 201) + `","examplePl":"To jest dom.","exampleRu":"Это дом.","meaningRu":"здание"}`,
			wantStatus: 400, wantBody: ruRule},
		{name: "501 letters in examplePl are rejected", sub: subOla, group: "rodzina",
			body:       `{"pl":"dom","ru":"дом","examplePl":"` + strings.Repeat("ż", 501) + `","exampleRu":"Это дом.","meaningRu":"здание"}`,
			wantStatus: 400, wantBody: examplePlRule},
		{name: "501 letters in exampleRu are rejected", sub: subOla, group: "rodzina",
			body:       `{"pl":"dom","ru":"дом","examplePl":"To jest dom.","exampleRu":"` + strings.Repeat("я", 501) + `","meaningRu":"здание"}`,
			wantStatus: 400, wantBody: exampleRuRule},
		{name: "201 letters in meaningRu are rejected", sub: subOla, group: "rodzina",
			body:       `{"pl":"dom","ru":"дом","examplePl":"To jest dom.","exampleRu":"Это дом.","meaningRu":"` + strings.Repeat("я", 201) + `"}`,
			wantStatus: 400, wantBody: meaningRuRule},
		{name: "a pl with a right-to-left override is rejected", sub: subOla, group: "rodzina",
			body:       `{"pl":"dom\u202emod","ru":"дом","examplePl":"To jest dom.","exampleRu":"Это дом.","meaningRu":"здание"}`,
			wantStatus: 400, wantBody: plRule},
		{name: "a blank ru is rejected", sub: subOla, group: "rodzina",
			body:       `{"pl":"dom","ru":"   ","examplePl":"To jest dom.","exampleRu":"Это дом.","meaningRu":"здание"}`,
			wantStatus: 400, wantBody: ruRule},
		{name: "a body without examplePl is rejected", sub: subOla, group: "rodzina",
			body:       `{"pl":"dom","ru":"дом","exampleRu":"Это дом.","meaningRu":"здание"}`,
			wantStatus: 400, wantBody: examplePlRule},
		{name: "a meaningRu with a line break is rejected", sub: subOla, group: "rodzina",
			body:       `{"pl":"dom","ru":"дом","examplePl":"To jest dom.","exampleRu":"Это дом.","meaningRu":"здание\nдом"}`,
			wantStatus: 400, wantBody: meaningRuRule},
		{name: "8 alternatives pass", sub: subOla, group: "rodzina",
			body:       `{"pl":"dom","ru":"дом","examplePl":"To jest dom.","exampleRu":"Это дом.","meaningRu":"здание","alternativesPl":["a1","a2","a3","a4","a5","a6","a7","a8"]}`,
			wantStatus: 201, want: with(func(c *Card) { c.AlternativesPl = []string{"a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8"} })},
		{name: "9 alternatives are rejected", sub: subOla, group: "rodzina",
			body:       `{"pl":"dom","ru":"дом","examplePl":"To jest dom.","exampleRu":"Это дом.","meaningRu":"здание","alternativesPl":["a1","a2","a3","a4","a5","a6","a7","a8","a9"]}`,
			wantStatus: 400, wantBody: altPlRule},
		{name: "200 letters in a term pass", sub: subOla, group: "rodzina",
			body:       `{"pl":"dom","ru":"дом","examplePl":"To jest dom.","exampleRu":"Это дом.","meaningRu":"здание","alternativesPl":["` + strings.Repeat("ż", 200) + `"]}`,
			wantStatus: 201, want: with(func(c *Card) { c.AlternativesPl = []string{strings.Repeat("ż", 200)} })},
		{name: "201 letters in a term are rejected", sub: subOla, group: "rodzina",
			body:       `{"pl":"dom","ru":"дом","examplePl":"To jest dom.","exampleRu":"Это дом.","meaningRu":"здание","alternativesPl":["` + strings.Repeat("ż", 201) + `"]}`,
			wantStatus: 400, wantBody: altPlRule},
		{name: "a blank alternative is rejected", sub: subOla, group: "rodzina",
			body:       `{"pl":"dom","ru":"дом","examplePl":"To jest dom.","exampleRu":"Это дом.","meaningRu":"здание","alternativesRu":[" "]}`,
			wantStatus: 400, wantBody: altRuRule},
		{name: "the first broken rule decides the text", sub: subOla, group: "rodzina",
			body:       `{"pl":"","ru":"","examplePl":"To jest dom.","exampleRu":"Это дом.","meaningRu":"здание"}`,
			wantStatus: 400, wantBody: plRule},
		{name: "a body that is not JSON is rejected", sub: subOla, group: "rodzina", body: `{"pl":`, wantStatus: 400, wantBody: notJSON},
		{name: "a member of another group is refused", sub: subEwa, group: "rodzina", body: baseBody, wantStatus: 403, wantBody: notMember},
		{name: "a user without a group is refused", sub: subJan, group: "rodzina", body: baseBody, wantStatus: 403, wantBody: notMember},
		{name: "a group ID that is not a UUID is refused", sub: subOla, group: "nope", body: baseBody, wantStatus: 403, wantBody: notMember},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			pool, srv, rodzina, _ := groupFixture(t)
			groupID := tt.group
			if groupID == "rodzina" {
				groupID = rodzina
			}

			resp, err := srv.Handle(ctx, groupRequest("POST /v1/groups/{groupId}/cards", tt.sub, tt.body, map[string]string{"groupId": groupID}))
			if err != nil {
				t.Fatalf("Handle returned error %v", err)
			}
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d (body %s)", resp.StatusCode, tt.wantStatus, resp.Body)
			}
			if tt.wantBody != "" {
				if resp.Body != tt.wantBody {
					t.Errorf("body = %s, want %s", resp.Body, tt.wantBody)
				}
				if n := countRows(t, pool, "cards"); n != 0 {
					t.Errorf("cards = %d, want 0", n)
				}
				return
			}
			var card Card
			if err := json.Unmarshal([]byte(resp.Body), &card); err != nil {
				t.Fatalf("body %s: %v", resp.Body, err)
			}
			if card.Id == "" {
				t.Fatalf("card = %+v, want an ID", card)
			}
			got := card
			got.Id = ""
			// reflect.DeepEqual treats a nil list and an empty list as different.
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("card = %+v, want %+v", got, tt.want)
			}
			var row Card
			var rowGroup string
			var archived bool
			if err := pool.QueryRow(ctx, "SELECT group_id, kind, pl, ru, example_pl, example_ru, meaning_ru, alternatives_pl, alternatives_ru, archived FROM cards WHERE id = $1", card.Id).
				Scan(&rowGroup, &row.Kind, &row.Pl, &row.Ru, &row.ExamplePl, &row.ExampleRu, &row.MeaningRu, &row.AlternativesPl, &row.AlternativesRu, &archived); err != nil {
				t.Fatalf("card %q: %v", card.Id, err)
			}
			if rowGroup != rodzina || archived || !reflect.DeepEqual(row, tt.want) {
				t.Errorf("row = %s %+v archived %t, want %s %+v archived false", rowGroup, row, archived, rodzina, tt.want)
			}
			if n := countRows(t, pool, "cards"); n != 1 {
				t.Errorf("cards = %d, want 1", n)
			}
		})
	}
}

func TestListCards(t *testing.T) {
	dom := func(id string) Card {
		return Card{Id: id, Kind: Translation, Pl: "dom", Ru: "дом", ExamplePl: "To jest dom.", ExampleRu: "Это дом.", MeaningRu: "здание", AlternativesPl: []string{}, AlternativesRu: []string{}}
	}
	tests := []struct {
		name       string
		setup      string // the SQL text that adds cards to the fixture; it finds a group by its name
		sub        string
		wantStatus int
		wantBody   string   // empty when the answer is checked by wantPl or want
		wantPl     []string // the pl values of the answer in order
		want       []Card   // the full cards, when the row checks all fields
	}{
		{name: "a group without cards has an empty list", sub: subOla, wantStatus: 200, wantBody: `{"cards":[]}`},
		// Polish order: ą is a letter after a, ł after l, and ż after z. Byte order gives adam, lato, zamek, Ąb, łódź, żaba.
		{name: "lists the cards in the Polish order", sub: subAdam, wantStatus: 200,
			setup: `INSERT INTO cards (group_id, pl, ru, example_pl, example_ru, meaning_ru)
				SELECT (SELECT id FROM groups WHERE name = v.grp), v.pl, 'ru', 'example', 'example', 'meaning'
				FROM (VALUES ('Rodzina', 'żaba'), ('Rodzina', 'zamek'), ('Rodzina', 'Ąb'), ('Rodzina', 'adam'), ('Rodzina', 'łódź'), ('Rodzina', 'lato'), ('Praca', 'obcy')) AS v (grp, pl)`,
			wantPl: []string{"adam", "Ąb", "lato", "łódź", "zamek", "żaba"}},
		{name: "archived cards are not in the list", sub: subOla, wantStatus: 200,
			setup: `INSERT INTO cards (group_id, pl, ru, example_pl, example_ru, meaning_ru, archived)
				SELECT (SELECT id FROM groups WHERE name = 'Rodzina'), v.pl, 'ru', 'example', 'example', 'meaning', v.archived
				FROM (VALUES ('dom', false), ('stary', true)) AS v (pl, archived)`,
			wantPl: []string{"dom"}},
		// After ANALYZE, PostgreSQL reads the small table in insert order, so the rows come in the reverse order of their IDs.
		// https://www.postgresql.org/docs/16/sql-analyze.html
		{name: "cards with one term are in the order of their IDs", sub: subOla, wantStatus: 200,
			setup: `INSERT INTO cards (group_id, id, pl, ru, example_pl, example_ru, meaning_ru)
				SELECT (SELECT id FROM groups WHERE name = 'Rodzina'), v.id, 'dom', 'дом', 'To jest dom.', 'Это дом.', 'здание'
				FROM (VALUES ('00000000-0000-0000-0000-000000000002'::uuid), ('00000000-0000-0000-0000-000000000001'::uuid)) AS v (id); ANALYZE cards`,
			want: []Card{dom("00000000-0000-0000-0000-000000000001"), dom("00000000-0000-0000-0000-000000000002")}},
		{name: "the card has all its fields", sub: subOla, wantStatus: 200,
			setup: `INSERT INTO cards (group_id, id, pl, ru, example_pl, example_ru, meaning_ru, alternatives_pl, alternatives_ru)
				VALUES ((SELECT id FROM groups WHERE name = 'Rodzina'), '00000000-0000-0000-0000-000000000001', 'źródło', 'источник', 'To jest źródło.', 'Это источник.', 'начало реки', '["zdrój"]', '[]')`,
			want: []Card{{Id: "00000000-0000-0000-0000-000000000001", Kind: Translation, Pl: "źródło", Ru: "источник", ExamplePl: "To jest źródło.", ExampleRu: "Это источник.",
				MeaningRu: "начало реки", AlternativesPl: []string{"zdrój"}, AlternativesRu: []string{}}}},
		{name: "a member of another group is refused", sub: subEwa, wantStatus: 403, wantBody: notMember},
		{name: "a user without a group is refused", sub: subJan, wantStatus: 403, wantBody: notMember},
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

			resp, err := srv.Handle(ctx, groupRequest("GET /v1/groups/{groupId}/cards", tt.sub, "", map[string]string{"groupId": rodzina}))
			if err != nil {
				t.Fatalf("Handle returned error %v", err)
			}
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d (body %s)", resp.StatusCode, tt.wantStatus, resp.Body)
			}
			if tt.wantBody != "" {
				if resp.Body != tt.wantBody {
					t.Errorf("body = %s, want %s", resp.Body, tt.wantBody)
				}
				return
			}
			var list Cards
			if err := json.Unmarshal([]byte(resp.Body), &list); err != nil {
				t.Fatalf("body %s: %v", resp.Body, err)
			}
			if tt.want != nil {
				// reflect.DeepEqual treats a nil list and an empty list as different.
				if !reflect.DeepEqual(list.Cards, tt.want) {
					t.Errorf("cards = %+v, want %+v", list.Cards, tt.want)
				}
				return
			}
			var pl []string
			for _, c := range list.Cards {
				pl = append(pl, c.Pl)
			}
			if !slices.Equal(pl, tt.wantPl) {
				t.Errorf("pl = %q, want %q", pl, tt.wantPl)
			}
		})
	}
}

func TestPatchCard(t *testing.T) {
	with := func(change func(*Card)) Card {
		c := cardK1
		change(&c)
		return c
	}
	tests := []struct {
		name       string
		sub        string
		group      string // the group ID of the path; "rodzina" for the ID of Rodzina
		card       string // the card ID of the path
		body       string
		wantStatus int
		wantBody   string // empty when the answer is the changed card
		want       Card   // the changed card
	}{
		{name: "changes one text and keeps the other fields", sub: subOla, group: "rodzina", card: cardK1.Id, body: `{"pl":"  chata  "}`,
			wantStatus: 200, want: with(func(c *Card) { c.Pl = "chata" })},
		{name: "changes all fields", sub: subAdam, group: "rodzina", card: cardK1.Id,
			body:       `{"pl":" zamek ","ru":" замок ","examplePl":" To jest zamek. ","exampleRu":" Это замок. ","meaningRu":" крепость ","alternativesPl":[" zamczysko "],"alternativesRu":["твердыня","дворец"]}`,
			wantStatus: 200, want: Card{Id: cardK1.Id, Kind: Translation, Pl: "zamek", Ru: "замок", ExamplePl: "To jest zamek.", ExampleRu: "Это замок.", MeaningRu: "крепость",
				AlternativesPl: []string{"zamczysko"}, AlternativesRu: []string{"твердыня", "дворец"}}},
		{name: "an empty body changes nothing", sub: subOla, group: "rodzina", card: cardK1.Id, body: `{}`, wantStatus: 200, want: cardK1},
		{name: "an empty list removes the terms", sub: subOla, group: "rodzina", card: cardK1.Id, body: `{"alternativesPl":[]}`,
			wantStatus: 200, want: with(func(c *Card) { c.AlternativesPl = []string{} })},
		{name: "each text at its maximum passes", sub: subOla, group: "rodzina", card: cardK1.Id,
			body: `{"pl":"` + strings.Repeat("ż", 200) + `","ru":"` + strings.Repeat("я", 200) + `","examplePl":"` + strings.Repeat("ż", 500) +
				`","exampleRu":"` + strings.Repeat("я", 500) + `","meaningRu":"` + strings.Repeat("я", 200) + `","alternativesPl":["` + strings.Repeat("ż", 200) + `"]}`,
			wantStatus: 200, want: with(func(c *Card) {
				c.Pl, c.Ru, c.ExamplePl = strings.Repeat("ż", 200), strings.Repeat("я", 200), strings.Repeat("ż", 500)
				c.ExampleRu, c.MeaningRu = strings.Repeat("я", 500), strings.Repeat("я", 200)
				c.AlternativesPl = []string{strings.Repeat("ż", 200)}
			})},
		{name: "201 letters in pl are rejected", sub: subOla, group: "rodzina", card: cardK1.Id, body: `{"pl":"` + strings.Repeat("ż", 201) + `"}`,
			wantStatus: 400, wantBody: plRule},
		{name: "a blank ru is rejected", sub: subOla, group: "rodzina", card: cardK1.Id, body: `{"ru":"   "}`, wantStatus: 400, wantBody: ruRule},
		{name: "501 letters in examplePl are rejected", sub: subOla, group: "rodzina", card: cardK1.Id, body: `{"examplePl":"` + strings.Repeat("ż", 501) + `"}`,
			wantStatus: 400, wantBody: examplePlRule},
		{name: "an exampleRu with a line break is rejected", sub: subOla, group: "rodzina", card: cardK1.Id, body: `{"exampleRu":"Это\nдом."}`,
			wantStatus: 400, wantBody: exampleRuRule},
		{name: "201 letters in meaningRu are rejected", sub: subOla, group: "rodzina", card: cardK1.Id, body: `{"meaningRu":"` + strings.Repeat("я", 201) + `"}`,
			wantStatus: 400, wantBody: meaningRuRule},
		{name: "9 alternatives are rejected", sub: subOla, group: "rodzina", card: cardK1.Id, body: `{"alternativesPl":["a1","a2","a3","a4","a5","a6","a7","a8","a9"]}`,
			wantStatus: 400, wantBody: altPlRule},
		{name: "a blank alternative is rejected", sub: subOla, group: "rodzina", card: cardK1.Id, body: `{"alternativesRu":[" "]}`, wantStatus: 400, wantBody: altRuRule},
		{name: "the first broken rule decides the text", sub: subOla, group: "rodzina", card: cardK1.Id, body: `{"ru":"","pl":""}`, wantStatus: 400, wantBody: plRule},
		{name: "a body that is not JSON is rejected", sub: subOla, group: "rodzina", card: cardK1.Id, body: `{"pl":`, wantStatus: 400, wantBody: notJSON},
		{name: "an archived card is not found", sub: subOla, group: "rodzina", card: cardK2.Id, body: `{"pl":"nowy"}`, wantStatus: 404, wantBody: cardNotFound},
		{name: "a card of another group is not found", sub: subOla, group: "rodzina", card: cardK3.Id, body: `{"pl":"nowy"}`, wantStatus: 404, wantBody: cardNotFound},
		{name: "a card ID that is not a UUID is not found", sub: subOla, group: "rodzina", card: "nope", body: `{"pl":"nowy"}`, wantStatus: 404, wantBody: cardNotFound},
		{name: "an unknown card ID is not found", sub: subOla, group: "rodzina", card: "00000000-0000-0000-0000-000000000009", body: `{"pl":"nowy"}`,
			wantStatus: 404, wantBody: cardNotFound},
		{name: "a member of another group is refused", sub: subEwa, group: "rodzina", card: cardK1.Id, body: `{"pl":"nowy"}`, wantStatus: 403, wantBody: notMember},
		{name: "a user without a group is refused", sub: subJan, group: "rodzina", card: cardK1.Id, body: `{"pl":"nowy"}`, wantStatus: 403, wantBody: notMember},
		{name: "a group ID that is not a UUID is refused", sub: subOla, group: "nope", card: cardK1.Id, body: `{"pl":"nowy"}`, wantStatus: 403, wantBody: notMember},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			pool, srv, rodzina, praca := groupFixture(t)
			if _, err := pool.Exec(ctx, cardFixture); err != nil {
				t.Fatal(err)
			}
			groupID := tt.group
			if groupID == "rodzina" {
				groupID = rodzina
			}

			resp, err := srv.Handle(ctx, groupRequest("PATCH /v1/groups/{groupId}/cards/{cardId}", tt.sub, tt.body,
				map[string]string{"groupId": groupID, "cardId": tt.card}))
			if err != nil {
				t.Fatalf("Handle returned error %v", err)
			}
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d (body %s)", resp.StatusCode, tt.wantStatus, resp.Body)
			}
			k1 := cardK1
			if tt.wantBody != "" {
				if resp.Body != tt.wantBody {
					t.Errorf("body = %s, want %s", resp.Body, tt.wantBody)
				}
			} else {
				var card Card
				if err := json.Unmarshal([]byte(resp.Body), &card); err != nil {
					t.Fatalf("body %s: %v", resp.Body, err)
				}
				// reflect.DeepEqual treats a nil list and an empty list as different.
				if !reflect.DeepEqual(card, tt.want) {
					t.Errorf("card = %+v, want %+v", card, tt.want)
				}
				k1 = tt.want
			}
			want := []cardRow{{k1, rodzina, false}, {cardK2, rodzina, true}, {cardK3, praca, false}}
			if got := cardRows(t, pool); !reflect.DeepEqual(got, want) {
				t.Errorf("rows = %+v, want %+v", got, want)
			}
		})
	}
}

func TestArchiveCard(t *testing.T) {
	tests := []struct {
		name         string
		sub          string
		group        string // the group ID of the path; "rodzina" for the ID of Rodzina
		card         string // the card ID of the path
		wantStatus   int
		wantBody     string
		wantArchived []string // the IDs of the archived cards after the call, in ID order
	}{
		{name: "archives a card", sub: subOla, group: "rodzina", card: cardK1.Id, wantStatus: 204, wantArchived: []string{cardK1.Id, cardK2.Id}},
		{name: "an archived card is not found", sub: subAdam, group: "rodzina", card: cardK2.Id, wantStatus: 404, wantBody: cardNotFound, wantArchived: []string{cardK2.Id}},
		{name: "a card of another group is not found", sub: subOla, group: "rodzina", card: cardK3.Id, wantStatus: 404, wantBody: cardNotFound, wantArchived: []string{cardK2.Id}},
		{name: "a card ID that is not a UUID is not found", sub: subOla, group: "rodzina", card: "nope", wantStatus: 404, wantBody: cardNotFound, wantArchived: []string{cardK2.Id}},
		{name: "an unknown card ID is not found", sub: subOla, group: "rodzina", card: "00000000-0000-0000-0000-000000000009", wantStatus: 404, wantBody: cardNotFound,
			wantArchived: []string{cardK2.Id}},
		{name: "a member of another group is refused", sub: subEwa, group: "rodzina", card: cardK1.Id, wantStatus: 403, wantBody: notMember, wantArchived: []string{cardK2.Id}},
		{name: "a user without a group is refused", sub: subJan, group: "rodzina", card: cardK1.Id, wantStatus: 403, wantBody: notMember, wantArchived: []string{cardK2.Id}},
		{name: "a group ID that is not a UUID is refused", sub: subOla, group: "nope", card: cardK1.Id, wantStatus: 403, wantBody: notMember, wantArchived: []string{cardK2.Id}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			pool, srv, rodzina, _ := groupFixture(t)
			if _, err := pool.Exec(ctx, cardFixture); err != nil {
				t.Fatal(err)
			}
			groupID := tt.group
			if groupID == "rodzina" {
				groupID = rodzina
			}

			resp, err := srv.Handle(ctx, groupRequest("DELETE /v1/groups/{groupId}/cards/{cardId}", tt.sub, "",
				map[string]string{"groupId": groupID, "cardId": tt.card}))
			if err != nil {
				t.Fatalf("Handle returned error %v", err)
			}
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d (body %s)", resp.StatusCode, tt.wantStatus, resp.Body)
			}
			if resp.Body != tt.wantBody {
				t.Errorf("body = %q, want %q", resp.Body, tt.wantBody)
			}
			var archived []string
			var cards []Card
			for _, r := range cardRows(t, pool) {
				if r.archived {
					archived = append(archived, r.card.Id)
				}
				cards = append(cards, r.card)
			}
			if !slices.Equal(archived, tt.wantArchived) {
				t.Errorf("archived = %q, want %q", archived, tt.wantArchived)
			}
			// An archive changes only archived. reflect.DeepEqual treats a nil list and an empty list as different.
			if want := []Card{cardK1, cardK2, cardK3}; !reflect.DeepEqual(cards, want) {
				t.Errorf("cards = %+v, want %+v", cards, want)
			}
		})
	}
}
