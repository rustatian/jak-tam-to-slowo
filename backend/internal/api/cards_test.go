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
	limitRule     = `{"message":"limit must be a whole number of 1 or more"}`
	afterRule     = `{"message":"invalid after"}`
)

// cardFixture adds the cards K1 and K2 (archived) to Rodzina and K3 to Praca of groupFixture.
const cardFixture = `INSERT INTO cards (group_id, id, pl, ru, example_pl, example_ru, meaning_ru, alternatives_pl, alternatives_ru, archived) VALUES
	((SELECT id FROM groups WHERE name = 'Rodzina'), '00000000-0000-0000-0000-000000000001', 'dom', 'дом', 'To jest dom.', 'Это дом.', 'здание', '["domek"]', '["жилище"]', false),
	((SELECT id FROM groups WHERE name = 'Rodzina'), '00000000-0000-0000-0000-000000000002', 'stary', 'старый', 'To jest stary dom.', 'Это старый дом.', 'не новый', '[]', '[]', true),
	((SELECT id FROM groups WHERE name = 'Praca'), '00000000-0000-0000-0000-000000000003', 'praca', 'работа', 'To jest praca.', 'Это работа.', 'труд', '[]', '[]', false)`

// The cards of cardFixture.
var (
	cardK1 = Card{Id: "00000000-0000-0000-0000-000000000001", Kind: CardKindTranslation, Pl: "dom", Ru: "дом", ExamplePl: "To jest dom.", ExampleRu: "Это дом.",
		MeaningRu: "здание", AlternativesPl: []string{"domek"}, AlternativesRu: []string{"жилище"}}
	cardK2 = Card{Id: "00000000-0000-0000-0000-000000000002", Kind: CardKindTranslation, Pl: "stary", Ru: "старый", ExamplePl: "To jest stary dom.", ExampleRu: "Это старый дом.",
		MeaningRu: "не новый", AlternativesPl: []string{}, AlternativesRu: []string{}}
	cardK3 = Card{Id: "00000000-0000-0000-0000-000000000003", Kind: CardKindTranslation, Pl: "praca", Ru: "работа", ExamplePl: "To jest praca.", ExampleRu: "Это работа.",
		MeaningRu: "труд", AlternativesPl: []string{}, AlternativesRu: []string{}}
)

// grammarBody is the create body of the grammar card of the tests. It has no ASCII apostrophe, because
// grammarFixture puts it in an SQL literal.
const grammarBody = `{"kind":"grammar","pl":"psy","ru":"","examplePl":"Na podwórku biegają dwa ___.","exampleRu":"","meaningRu":"","alternativesPl":[],"alternativesRu":[],"grammar":{"topic":"e ruchome","used":"dwa piesy","right":"dwa psy","hint":"(pies)","facts":["rzeczownik","rodzaj męski żywotny"],"slots":["Mianownik","liczba mnoga","niemęskoosobowy","końcówka -y"],"rule":"W liczbie mnogiej „pies” traci „e”: psy.","examples":["pies → psy","dom → domy","kobieta → kobiety"],"table":{"title":"Mianownik, liczba mnoga: końcówki według rodzaju","columns":["Rodzaj","Końcówka","Przykład"],"groups":[{"title":"W liczbie mnogiej: męskoosobowy","rows":[{"names":["męski osobowy"],"endings":"-i, -y, -owie, -e","examples":["student → studenci"],"hit":false}]},{"title":"W liczbie mnogiej: niemęskoosobowy","rows":[{"names":["męski żywotny","męski nieżywotny","żeński"],"endings":"-y, -i, -e","examples":["pies → psy","dom → domy","kobieta → kobiety"],"hit":true},{"names":["nijaki"],"endings":"-a","examples":["okno → okna"],"hit":false}]}]}}}`

// grammarCreate decodes grammarBody into a new card, so no two cards share a slice. It applies change when change is
// not nil.
func grammarCreate(change func(c *CardCreate)) CardCreate {
	var c CardCreate
	if err := json.Unmarshal([]byte(grammarBody), &c); err != nil {
		panic(err)
	}
	if change != nil {
		change(&c)
	}
	return c
}

// grammarRows gives n rows of a grammar table, each with one name and hit false.
func grammarRows(n int) []GrammarRow {
	rows := make([]GrammarRow, n)
	for i := range rows {
		rows[i] = GrammarRow{Names: []string{"x"}, Endings: "-x", Examples: []string{"x"}}
	}
	return rows
}

// grammarJSON is grammarCreate(change) as a create body. A nil list is null in the body. A field with omitempty is not
// in the body when it is nil or empty.
func grammarJSON(change func(c *CardCreate)) string {
	b, err := json.Marshal(grammarCreate(change), json.FormatNilSliceAsNull(true))
	if err != nil {
		panic(err)
	}
	return string(b)
}

// grammarFixture adds the grammar card G1 of grammarBody to Rodzina of groupFixture.
const grammarFixture = `INSERT INTO cards (group_id, id, kind, pl, ru, example_pl, example_ru, meaning_ru, alternatives_pl, alternatives_ru, grammar)
	SELECT (SELECT id FROM groups WHERE name = 'Rodzina'), '00000000-0000-0000-0000-000000000011', 'grammar', b->>'pl', '', b->>'examplePl', '', '', '[]', '[]', b->'grammar'
	FROM (VALUES ('` + grammarBody + `'::jsonb)) AS v (b)`

// cardG1 is the card of grammarFixture, with empty Russian texts. It is a function, because Card.Grammar is a pointer:
// with a shared value, the change of one row would stay for the next rows.
func cardG1() Card {
	return Card{Id: "00000000-0000-0000-0000-000000000011", Kind: CardKindGrammar, Pl: "psy", ExamplePl: "Na podwórku biegają dwa ___.",
		AlternativesPl: []string{}, AlternativesRu: []string{}, Grammar: &Grammar{Topic: "e ruchome", Used: "dwa piesy", Right: "dwa psy", Hint: "(pies)",
			Facts: []string{"rzeczownik", "rodzaj męski żywotny"}, Slots: []string{"Mianownik", "liczba mnoga", "niemęskoosobowy", "końcówka -y"},
			Rule: "W liczbie mnogiej „pies” traci „e”: psy.", Examples: []string{"pies → psy", "dom → domy", "kobieta → kobiety"},
			Table: &GrammarTable{Title: "Mianownik, liczba mnoga: końcówki według rodzaju", Columns: []string{"Rodzaj", "Końcówka", "Przykład"}, Groups: []GrammarGroup{
				{Title: new("W liczbie mnogiej: męskoosobowy"), Rows: []GrammarRow{
					{Names: []string{"męski osobowy"}, Endings: "-i, -y, -owie, -e", Examples: []string{"student → studenci"}},
				}},
				{Title: new("W liczbie mnogiej: niemęskoosobowy"), Rows: []GrammarRow{
					{Names: []string{"męski żywotny", "męski nieżywotny", "żeński"}, Endings: "-y, -i, -e", Examples: []string{"pies → psy", "dom → domy", "kobieta → kobiety"}, Hit: true},
					{Names: []string{"nijaki"}, Endings: "-a", Examples: []string{"okno → okna"}},
				}},
			}}}}
}

// cardRow is a row of the table cards.
type cardRow struct {
	card     Card
	group    string
	archived bool
}

// cardRows reads all card rows in ID order. Without ORDER BY, the order of the rows is not specified.
// https://www.postgresql.org/docs/16/queries-order.html
func cardRows(t *testing.T, pool *pgxpool.Pool) []cardRow {
	t.Helper()
	rows, err := pool.Query(context.Background(), "SELECT id, kind, pl, ru, example_pl, example_ru, meaning_ru, alternatives_pl, alternatives_ru, grammar, group_id, archived FROM cards ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	got, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (cardRow, error) {
		var r cardRow
		c := &r.card
		err := row.Scan(&c.Id, &c.Kind, &c.Pl, &c.Ru, &c.ExamplePl, &c.ExampleRu, &c.MeaningRu, &c.AlternativesPl, &c.AlternativesRu, &c.Grammar, &r.group, &r.archived)
		return r, err
	})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestCreateCard(t *testing.T) {
	const baseBody = `{"pl":"dom","ru":"дом","examplePl":"To jest dom.","exampleRu":"Это дом.","meaningRu":"здание"}`
	base := Card{Kind: CardKindTranslation, Pl: "dom", Ru: "дом", ExamplePl: "To jest dom.", ExampleRu: "Это дом.", MeaningRu: "здание", AlternativesPl: []string{}, AlternativesRu: []string{}}
	with := func(change func(*Card)) Card {
		c := base
		change(&c)
		return c
	}
	// g1 is the card G1 without its ID. It applies change when change is not nil.
	g1 := func(change func(*Card)) Card {
		c := cardG1()
		c.Id = ""
		if change != nil {
			change(&c)
		}
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
		{name: "creates a grammar card", sub: subOla, group: "rodzina",
			body:       grammarJSON(func(c *CardCreate) { c.Pl, c.Grammar.Hint = "  psy  ", " (pies) " }),
			wantStatus: 201, want: g1(nil)},
		{name: "a grammar card without the lists that can be empty", sub: subOla, group: "rodzina",
			body: grammarJSON(func(c *CardCreate) {
				c.AlternativesPl, c.AlternativesRu, c.Grammar.Facts, c.Grammar.Slots = nil, nil, nil, nil
			}),
			wantStatus: 201, want: g1(func(c *Card) { c.Grammar.Facts, c.Grammar.Slots = []string{}, []string{} })},
		{name: "a grammar card with a Russian term is rejected", sub: subOla, group: "rodzina", body: grammarJSON(func(c *CardCreate) { c.Ru = "Собака" }),
			wantStatus: 400, wantBody: `{"message":"ru, exampleRu and meaningRu must be empty for a grammar card"}`},
		{name: "a grammar card without grammar is rejected", sub: subOla, group: "rodzina", body: grammarJSON(func(c *CardCreate) { c.Grammar = nil }),
			wantStatus: 400, wantBody: `{"message":"grammar is required for a grammar card"}`},
		{name: "a grammar card without a gap is rejected", sub: subOla, group: "rodzina",
			body:       grammarJSON(func(c *CardCreate) { c.ExamplePl = "Na podwórku biegają dwa psy." }),
			wantStatus: 400, wantBody: `{"message":"examplePl must have ___ exactly once"}`},
		// The Russian texts stay empty, and checkCard refuses an empty ru. So the message also shows that this rule
		// comes before checkCard.
		{name: "a translation card with grammar is rejected", sub: subOla, group: "rodzina", body: grammarJSON(func(c *CardCreate) { c.Kind = nil }),
			wantStatus: 400, wantBody: `{"message":"only a grammar card can have grammar"}`},
		{name: "the kind translation gives a translation card", sub: subOla, group: "rodzina",
			body:       `{"kind":"translation","pl":"dom","ru":"дом","examplePl":"To jest dom.","exampleRu":"Это дом.","meaningRu":"здание"}`,
			wantStatus: 201, want: base},
		{name: "an unknown kind is rejected", sub: subOla, group: "rodzina",
			body:       `{"kind":"vocabulary","pl":"dom","ru":"дом","examplePl":"To jest dom.","exampleRu":"Это дом.","meaningRu":"здание"}`,
			wantStatus: 400, wantBody: `{"message":"kind must be translation or grammar"}`},
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
			if err := pool.QueryRow(ctx, "SELECT group_id, kind, pl, ru, example_pl, example_ru, meaning_ru, alternatives_pl, alternatives_ru, grammar, archived FROM cards WHERE id = $1", card.Id).
				Scan(&rowGroup, &row.Kind, &row.Pl, &row.Ru, &row.ExamplePl, &row.ExampleRu, &row.MeaningRu, &row.AlternativesPl, &row.AlternativesRu, &row.Grammar, &archived); err != nil {
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
		return Card{Id: id, Kind: CardKindTranslation, Pl: "dom", Ru: "дом", ExamplePl: "To jest dom.", ExampleRu: "Это дом.", MeaningRu: "здание", AlternativesPl: []string{}, AlternativesRu: []string{}}
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
			want: []Card{{Id: "00000000-0000-0000-0000-000000000001", Kind: CardKindTranslation, Pl: "źródło", Ru: "источник", ExamplePl: "To jest źródło.", ExampleRu: "Это источник.",
				MeaningRu: "начало реки", AlternativesPl: []string{"zdrój"}, AlternativesRu: []string{}}}},
		{name: "a grammar card has its grammar fields", sub: subOla, wantStatus: 200, setup: grammarFixture, want: []Card{cardG1()}},
		{name: "a card with an empty ru keeps its place in the list", sub: subOla, wantStatus: 200,
			setup: `INSERT INTO cards (group_id, pl, ru, example_pl, example_ru, meaning_ru)
				SELECT (SELECT id FROM groups WHERE name = 'Rodzina'), v.pl, v.ru, 'example', 'example', 'meaning'
				FROM (VALUES ('żaba', ''), ('dom', 'дом'), ('ala', 'аля')) AS v (pl, ru)`,
			wantPl: []string{"ala", "dom", "żaba"}},
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

func TestSearchKey(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "upper case becomes lower case", in: "Dom", want: "dom"},
		{name: "Polish letters lose their marks", in: "Zażółć gęślą jaźń", want: "zazolc gesla jazn"},
		{name: "upper-case Polish letters lose their marks", in: "ŁÓDŹ", want: "lodz"},
		{name: "ё becomes е", in: "Ёлка", want: "елка"},
		{name: "й stays й", in: "Чай", want: "чай"},
		{name: "runs of white space become one space", in: "  dobry \t wieczór  ", want: "dobry wieczor"},
		{name: "a non-breaking space is white space", in: "dobry\u00a0wieczór", want: "dobry wieczor"},
		{name: "a decomposed Polish letter", in: "mo\u0301j", want: "moj"},
		{name: "a decomposed ё", in: "е\u0308лка", want: "елка"},
		{name: "a decomposed й stays й", in: "чаи\u0306", want: "ча\u0439"},
		{name: "punctuation stays", in: "Co to jest?", want: "co to jest?"},
		{name: "only white space gives an empty key", in: "   ", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := searchKey(tt.in); got != tt.want {
				t.Errorf("searchKey(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestAnswerKey(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "Polish letters keep their marks", in: "Zażółć gęślą jaźń", want: "zażółć gęślą jaźń"},
		{name: "upper-case Polish letters keep their marks", in: "ŁÓDŹ", want: "łódź"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := answerKey(tt.in); got != tt.want {
				t.Errorf("answerKey(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSearchCards(t *testing.T) {
	// searchFixture adds the cards 0001 to 0010 to the groups of groupFixture. 0006 is archived, and 0007 is in Praca.
	const searchFixture = `INSERT INTO cards (group_id, id, pl, ru, example_pl, example_ru, meaning_ru, alternatives_pl, alternatives_ru, archived)
		SELECT (SELECT id FROM groups WHERE name = v.grp), v.id::uuid, v.pl, v.ru, 'p', 'r', 'm', v.alt_pl::jsonb, v.alt_ru::jsonb, v.archived
		FROM (VALUES
			('Rodzina', '00000000-0000-0000-0000-000000000001', 'wieczór', 'вечер', '[]', '[]', false),
			('Rodzina', '00000000-0000-0000-0000-000000000002', 'dobry wieczór', 'добрый вечер', '[]', '[]', false),
			('Rodzina', '00000000-0000-0000-0000-000000000003', 'wieczorek', 'вечерок', '[]', '[]', false),
			('Rodzina', '00000000-0000-0000-0000-000000000004', 'żółw', 'черепаха', '[]', '[]', false),
			('Rodzina', '00000000-0000-0000-0000-000000000005', 'jeż', 'ёж', '[]', '[]', false),
			('Rodzina', '00000000-0000-0000-0000-000000000006', 'noc', 'вечер и ночь', '[]', '[]', true),
			('Praca', '00000000-0000-0000-0000-000000000007', 'wieczór', 'вечер', '[]', '[]', false),
			('Rodzina', '00000000-0000-0000-0000-000000000008', 'zmierzch', 'сумерки', '["wieczór"]', '["вечер"]', false),
			('Rodzina', '00000000-0000-0000-0000-000000000009', 'to jest ok', 'ok', '[]', '[]', false),
			('Rodzina', '00000000-0000-0000-0000-000000000010', 'oko', 'глаз', '[]', '[]', false)
		) AS v (grp, id, pl, ru, alt_pl, alt_ru, archived)`
	// Polish order: ó is a letter after o, and ż after z.
	all := []string{"dobry wieczór", "jeż", "oko", "psy", "to jest ok", "wieczorek", "wieczór", "zmierzch", "żółw"}
	tests := []struct {
		name       string
		sub        string
		query      string // the raw query string of the request
		wantStatus int
		wantBody   string   // empty when the answer is checked by wantPl
		wantPl     []string // the pl values of the answer in order
	}{
		{name: "an empty q lists all cards", sub: subOla, query: "q=", wantStatus: 200, wantPl: all},
		{name: "a q of spaces lists all cards", sub: subOla, query: "q=%20%20", wantStatus: 200, wantPl: all},
		{name: "a query without Polish marks", sub: subOla, query: "q=wieczor", wantStatus: 200, wantPl: []string{"wieczór", "wieczorek", "dobry wieczór"}},
		// WIECZÓR
		{name: "a query with Polish marks and upper case", sub: subAdam, query: "q=WIECZ%C3%93R", wantStatus: 200,
			wantPl: []string{"wieczór", "wieczorek", "dobry wieczór"}},
		// вечер
		{name: "a Russian query", sub: subOla, query: "q=%D0%B2%D0%B5%D1%87%D0%B5%D1%80", wantStatus: 200, wantPl: []string{"wieczór", "wieczorek", "dobry wieczór"}},
		// еж
		{name: "ё and е are equal", sub: subOla, query: "q=%D0%B5%D0%B6", wantStatus: 200, wantPl: []string{"jeż"}},
		{name: "cards of one rank are in the order of pl, not of the key", sub: subOla, query: "q=czor", wantStatus: 200,
			wantPl: []string{"dobry wieczór", "wieczorek", "wieczór"}},
		// The ru of "to jest ok" is equal to the query, and "oko" starts with it.
		{name: "the better rank of the two terms counts", sub: subOla, query: "q=ok", wantStatus: 200, wantPl: []string{"to jest ok", "oko"}},
		{name: "a grammar card is found by its answer", sub: subOla, query: "q=psy", wantStatus: 200, wantPl: []string{"psy"}},
		// %C5%BC is the letter ż.
		{name: "a q of 200 letters passes", sub: subOla, query: "q=" + strings.Repeat("%C5%BC", 200), wantStatus: 200, wantBody: `{"cards":[]}`},
		{name: "a q of 201 letters is rejected", sub: subOla, query: "q=" + strings.Repeat("%C5%BC", 201), wantStatus: 400,
			wantBody: `{"message":"q must have at most 200 characters"}`},
		{name: "a limit of 0 is rejected", sub: subOla, query: "limit=0", wantStatus: 400, wantBody: limitRule},
		{name: "a negative limit is rejected", sub: subOla, query: "limit=-1", wantStatus: 400, wantBody: limitRule},
		{name: "a limit that is not a whole number is rejected", sub: subOla, query: "limit=1.5", wantStatus: 400, wantBody: limitRule},
		{name: "an empty limit is rejected", sub: subOla, query: "limit=", wantStatus: 400, wantBody: limitRule},
		// a%2Bb is the text a+b. Base64url has no +.
		{name: "an after that is not base64url is rejected", sub: subOla, query: "after=a%2Bb", wantStatus: 400, wantBody: afterRule},
		// bm90IGpzb24 is "not json" in base64url.
		{name: "an after that is not JSON is rejected", sub: subOla, query: "after=bm90IGpzb24", wantStatus: 400, wantBody: afterRule},
		// eyJyYW5rIjoieCJ9 is {"rank":"x"} in base64url.
		{name: "an after with a wrong field type is rejected", sub: subOla, query: "after=eyJyYW5rIjoieCJ9", wantStatus: 400, wantBody: afterRule},
		{name: "a query string that is not valid is rejected", sub: subOla, query: "q=%zz", wantStatus: 400, wantBody: `{"message":"invalid query string"}`},
		{name: "a member of another group is refused before the query is read", sub: subEwa, query: "q=%zz", wantStatus: 403, wantBody: notMember},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			pool, srv, rodzina, _ := groupFixture(t)
			if _, err := pool.Exec(ctx, searchFixture+"; "+grammarFixture); err != nil {
				t.Fatal(err)
			}
			req := groupRequest("GET /v1/groups/{groupId}/cards", tt.sub, "", map[string]string{"groupId": rodzina})
			req.RawQueryString = tt.query

			resp, err := srv.Handle(ctx, req)
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

func TestListCardPages(t *testing.T) {
	// pagesFixture adds six cards to Rodzina of groupFixture. Polish order: adam, Ąb, lato, łódź, zamek, żaba. Byte
	// order: adam, lato, zamek, Ąb, łódź, żaba.
	const pagesFixture = `INSERT INTO cards (group_id, pl, ru, example_pl, example_ru, meaning_ru)
		SELECT (SELECT id FROM groups WHERE name = 'Rodzina'), v.pl, 'ru', 'example', 'example', 'meaning'
		FROM (VALUES ('żaba'), ('zamek'), ('Ąb'), ('adam'), ('łódź'), ('lato')) AS v (pl)`
	// addCard is the SQL text that adds the card pl to Rodzina.
	addCard := func(pl string) string {
		return `INSERT INTO cards (group_id, pl, ru, example_pl, example_ru, meaning_ru)
			VALUES ((SELECT id FROM groups WHERE name = 'Rodzina'), '` + pl + `', 'ru', 'example', 'example', 'meaning')`
	}
	all := []string{"adam", "Ąb", "lato", "łódź", "zamek", "żaba"}
	pagesOf2 := [][]string{{"adam", "Ąb"}, {"lato", "łódź"}, {"zamek", "żaba"}}
	tests := []struct {
		name      string
		setup     string     // the SQL text that adds cards to the fixture
		query     string     // the raw query string of the first page
		between   string     // the SQL text that runs once, after the first page
		wantPages [][]string // the pl values of each page
	}{
		{name: "pages of 2 keep the Polish order", setup: pagesFixture, query: "limit=2", wantPages: pagesOf2},
		{name: "the last page has the rest", setup: pagesFixture, query: "limit=4", wantPages: [][]string{{"adam", "Ąb", "lato", "łódź"}, {"zamek", "żaba"}}},
		{name: "a limit equal to the number of cards gives one page", setup: pagesFixture, query: "limit=6", wantPages: [][]string{all}},
		{name: "a limit above the number of cards gives one page", setup: pagesFixture, query: "limit=100", wantPages: [][]string{all}},
		{name: "without limit the answer has all cards", setup: pagesFixture, wantPages: [][]string{all}},
		{name: "an empty group has one empty page", query: "limit=2", wantPages: [][]string{{}}},
		{name: "pages of a search keep the order of the rank", query: "q=wieczor&limit=2",
			setup: `INSERT INTO cards (group_id, pl, ru, example_pl, example_ru, meaning_ru)
				SELECT (SELECT id FROM groups WHERE name = 'Rodzina'), v.pl, 'ru', 'example', 'example', 'meaning'
				FROM (VALUES ('dobry wieczór'), ('wieczorek'), ('wieczór'), ('jeż')) AS v (pl)`,
			wantPages: [][]string{{"wieczór", "wieczorek"}, {"dobry wieczór"}}},
		{name: "cards with the same pl come in pages by ID", query: "limit=1",
			setup: `INSERT INTO cards (group_id, id, pl, ru, example_pl, example_ru, meaning_ru)
				SELECT (SELECT id FROM groups WHERE name = 'Rodzina'), v.id, 'dom', 'ru', 'example', 'example', 'meaning'
				FROM (VALUES ('00000000-0000-0000-0000-000000000001'::uuid), ('00000000-0000-0000-0000-000000000002'::uuid)) AS v (id)`,
			wantPages: [][]string{{"dom"}, {"dom"}}},
		{name: "a card archived on an earlier page does not move the next page", setup: pagesFixture, query: "limit=2",
			between: `UPDATE cards SET archived = true WHERE pl = 'adam'`, wantPages: pagesOf2},
		{name: "the archived last card of a page does not move the next page", setup: pagesFixture, query: "limit=2",
			between: `UPDATE cards SET archived = true WHERE pl = 'Ąb'`, wantPages: pagesOf2},
		{name: "a card added before the next value does not move the next page", setup: pagesFixture, query: "limit=2", between: addCard("aaa"), wantPages: pagesOf2},
		{name: "a card added after the next value comes on a later page", setup: pagesFixture, query: "limit=2", between: addCard("mama"),
			wantPages: [][]string{{"adam", "Ąb"}, {"lato", "łódź"}, {"mama", "zamek"}, {"żaba"}}},
		// For each ID, the key of „tak” has a + in standard base64 and a - in base64url.
		{name: "a next value needs no escape in a query string", setup: addCard("„tak”") + "; " + addCard("zzz"), query: "limit=1",
			wantPages: [][]string{{"„tak”"}, {"zzz"}}},
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
			var pages [][]string
			query := tt.query
			// The loop asks for at most one page more than wantPages: a next value on the last wanted page adds a page, so
			// the row fails.
			for i := range len(tt.wantPages) + 1 {
				req := groupRequest("GET /v1/groups/{groupId}/cards", subOla, "", map[string]string{"groupId": rodzina})
				req.RawQueryString = query

				resp, err := srv.Handle(ctx, req)
				if err != nil {
					t.Fatalf("Handle returned error %v", err)
				}
				if resp.StatusCode != 200 {
					t.Fatalf("page %d: status = %d, want 200 (body %s)", i+1, resp.StatusCode, resp.Body)
				}
				var list Cards
				if err := json.Unmarshal([]byte(resp.Body), &list); err != nil {
					t.Fatalf("body %s: %v", resp.Body, err)
				}
				var pl []string
				for _, c := range list.Cards {
					pl = append(pl, c.Pl)
				}
				pages = append(pages, pl)
				if i == 0 && tt.between != "" {
					if _, err := pool.Exec(ctx, tt.between); err != nil {
						t.Fatal(err)
					}
				}
				if list.Next == nil {
					break
				}
				// next goes into the query string without an escape, so a + in it becomes a space and the page fails.
				query = tt.query + "&after=" + *list.Next
			}
			if !slices.EqualFunc(pages, tt.wantPages, slices.Equal) {
				t.Errorf("pages = %q, want %q", pages, tt.wantPages)
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
		group      string // the group ID of the path; "rodzina" for the ID of Rodzina, "praca" for the ID of Praca
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
			wantStatus: 200, want: Card{Id: cardK1.Id, Kind: CardKindTranslation, Pl: "zamek", Ru: "замок", ExamplePl: "To jest zamek.", ExampleRu: "Это замок.", MeaningRu: "крепость",
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
		{name: "a grammar card cannot be changed", sub: subOla, group: "rodzina", card: cardG1().Id, body: `{"pl":"psa"}`,
			wantStatus: 409, wantBody: `{"message":"a grammar card cannot be changed"}`},
		{name: "the body is checked before the kind", sub: subOla, group: "rodzina", card: cardG1().Id, body: `{"pl":""}`, wantStatus: 400, wantBody: plRule},
		{name: "a grammar card of another group is not found", sub: subEwa, group: "praca", card: cardG1().Id, body: `{"pl":"psa"}`,
			wantStatus: 404, wantBody: cardNotFound},
		{name: "a member of another group is refused", sub: subEwa, group: "rodzina", card: cardK1.Id, body: `{"pl":"nowy"}`, wantStatus: 403, wantBody: notMember},
		{name: "a user without a group is refused", sub: subJan, group: "rodzina", card: cardK1.Id, body: `{"pl":"nowy"}`, wantStatus: 403, wantBody: notMember},
		{name: "a group ID that is not a UUID is refused", sub: subOla, group: "nope", card: cardK1.Id, body: `{"pl":"nowy"}`, wantStatus: 403, wantBody: notMember},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			pool, srv, rodzina, praca := groupFixture(t)
			if _, err := pool.Exec(ctx, cardFixture+"; "+grammarFixture); err != nil {
				t.Fatal(err)
			}
			groupID := tt.group
			switch groupID {
			case "rodzina":
				groupID = rodzina
			case "praca":
				groupID = praca
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
			want := []cardRow{{k1, rodzina, false}, {cardK2, rodzina, true}, {cardK3, praca, false}, {cardG1(), rodzina, false}}
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
		{name: "archives a grammar card", sub: subOla, group: "rodzina", card: cardG1().Id, wantStatus: 204, wantArchived: []string{cardK2.Id, cardG1().Id}},
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
			if _, err := pool.Exec(ctx, cardFixture+"; "+grammarFixture); err != nil {
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
			if want := []Card{cardK1, cardK2, cardK3, cardG1()}; !reflect.DeepEqual(cards, want) {
				t.Errorf("cards = %+v, want %+v", cards, want)
			}
		})
	}
}

func TestCheckGrammarCard(t *testing.T) {
	tests := []struct {
		name     string
		change   func(c *CardCreate) // the change of the fixture grammarCreate(nil)
		want     string              // the message; empty for a pass
		wantCard func(c *CardCreate) // the change of the fixture that gives the card after the check; nil when the row does not compare the card
	}{
		{name: "spaces at the ends of each text are removed", change: func(c *CardCreate) {
			pad := func(list []string) {
				for i := range list {
					list[i] = " " + list[i] + " "
				}
			}
			g, table := c.Grammar, c.Grammar.Table
			for _, s := range []*string{&c.Pl, &c.ExamplePl, &g.Topic, &g.Used, &g.Right, &g.Hint, &g.Rule, &table.Title, table.Groups[0].Title, table.Groups[1].Title} {
				*s = " " + *s + " "
			}
			pad(g.Facts)
			pad(g.Slots)
			pad(g.Examples)
			pad(table.Columns)
			for _, r := range []*GrammarRow{&table.Groups[0].Rows[0], &table.Groups[1].Rows[0], &table.Groups[1].Rows[1]} {
				pad(r.Names)
				r.Endings = " " + r.Endings + " "
				pad(r.Examples)
			}
			c.Ru, c.ExampleRu, c.MeaningRu = " ", " ", " "
		}, wantCard: func(*CardCreate) {}},
		{name: "the lists that the card does not have become empty lists", change: func(c *CardCreate) {
			c.AlternativesPl, c.AlternativesRu, c.Grammar.Facts, c.Grammar.Slots = nil, nil, nil, nil
		}, wantCard: func(c *CardCreate) {
			c.AlternativesPl, c.AlternativesRu, c.Grammar.Facts, c.Grammar.Slots = &[]string{}, &[]string{}, []string{}, []string{}
		}},
		{name: "each text at its maximum passes", change: func(c *CardCreate) {
			fill := func(list []string, n int) {
				for i := range list {
					list[i] = strings.Repeat("ż", n)
				}
			}
			g, table := c.Grammar, c.Grammar.Table
			c.Pl, c.ExamplePl, c.AlternativesPl = strings.Repeat("ż", 200), "___"+strings.Repeat("ż", 497), &[]string{strings.Repeat("ż", 200)}
			g.Topic, g.Used, g.Right, g.Hint, g.Rule = strings.Repeat("ż", 40), strings.Repeat("ż", 200), strings.Repeat("ź", 200), strings.Repeat("ó", 200), strings.Repeat("ż", 500)
			fill(g.Facts, 40)
			fill(g.Slots, 40)
			fill(g.Examples, 200)
			table.Title = strings.Repeat("ż", 100)
			fill(table.Columns, 40)
			table.Groups[0].Title, table.Groups[1].Title = new(strings.Repeat("ż", 80)), new(strings.Repeat("ż", 80))
			for _, r := range []*GrammarRow{&table.Groups[0].Rows[0], &table.Groups[1].Rows[0], &table.Groups[1].Rows[1]} {
				fill(r.Names, 40)
				r.Endings = strings.Repeat("ż", 60)
				fill(r.Examples, 80)
			}
		}},
		{name: "each list at its maximum passes", change: func(c *CardCreate) {
			c.AlternativesPl = &[]string{"a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8"}
			c.Grammar.Facts = []string{"a", "b", "c", "d"}
			c.Grammar.Slots = []string{"a", "b", "c", "d", "e"}
			table := c.Grammar.Table
			table.Groups = []GrammarGroup{{Rows: grammarRows(8)}, table.Groups[1], {Rows: grammarRows(1)}, {Rows: grammarRows(1)}}
		}},
		{name: "a card without a table passes", change: func(c *CardCreate) { c.Grammar.Table = nil }},
		{name: "a group without a title passes", change: func(c *CardCreate) { c.Grammar.Table.Groups[0].Title = nil }},
		{name: "two examples pass", change: func(c *CardCreate) { c.Grammar.Examples = []string{"pies → psy", "dom → domy"} }},
		{name: "a table with one group of one row passes", change: func(c *CardCreate) {
			c.Grammar.Table.Groups = []GrammarGroup{{Rows: []GrammarRow{c.Grammar.Table.Groups[1].Rows[0]}}}
		}},
		{name: "a hint with fewer words than used passes", change: func(c *CardCreate) {
			c.Grammar.Used, c.Grammar.Right, c.Grammar.Hint = "do sklep", "do sklepu", "(sklep)"
		}},
		{name: "a hint with the same words as used passes", change: func(c *CardCreate) {
			c.Grammar.Used, c.Grammar.Right, c.Grammar.Hint = "mój brat", "mojego brata", "(mój brat)"
		}},
		{name: "the words of used in another order pass", change: func(c *CardCreate) {
			c.Grammar.Used, c.Grammar.Right, c.Grammar.Hint = "mój brat", "mojego brata", "brat mój i siostra"
		}},
		{name: "a word that starts with used is another word", change: func(c *CardCreate) {
			c.Grammar.Used, c.Grammar.Right, c.Grammar.Hint = "pies", "psy", "(piesek, liczba mnoga)"
		}},
		{name: "201 letters in pl", change: func(c *CardCreate) { c.Pl = strings.Repeat("ż", 201) }, want: "pl must have 1 to 200 characters"},
		{name: "501 letters in examplePl", change: func(c *CardCreate) { c.ExamplePl = "___" + strings.Repeat("ż", 498) },
			want: "examplePl must have 1 to 500 characters"},
		{name: "a sentence without a gap", change: func(c *CardCreate) { c.ExamplePl = "Na podwórku biegają dwa psy." }, want: "examplePl must have ___ exactly once"},
		{name: "a sentence with two gaps", change: func(c *CardCreate) { c.ExamplePl = "___ biegają dwa ___." }, want: "examplePl must have ___ exactly once"},
		{name: "9 alternatives", change: func(c *CardCreate) {
			c.AlternativesPl = &[]string{"a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8", "a9"}
		}, want: "alternativesPl must have at most 8 terms of 1 to 200 characters"},
		{name: "a Russian term", change: func(c *CardCreate) { c.Ru = "Собака" }, want: "ru, exampleRu and meaningRu must be empty for a grammar card"},
		{name: "a Russian example", change: func(c *CardCreate) { c.ExampleRu = "Собака бежит." }, want: "ru, exampleRu and meaningRu must be empty for a grammar card"},
		{name: "a Russian meaning", change: func(c *CardCreate) { c.MeaningRu = "собака" }, want: "ru, exampleRu and meaningRu must be empty for a grammar card"},
		{name: "a Russian alternative", change: func(c *CardCreate) { c.AlternativesRu = &[]string{"собака"} }, want: "alternativesRu must be empty for a grammar card"},
		{name: "no grammar", change: func(c *CardCreate) { c.Grammar = nil }, want: "grammar is required for a grammar card"},
		{name: "41 letters in topic", change: func(c *CardCreate) { c.Grammar.Topic = strings.Repeat("ż", 41) }, want: "grammar.topic must have 1 to 40 characters"},
		{name: "201 letters in used", change: func(c *CardCreate) { c.Grammar.Used = strings.Repeat("ż", 201) }, want: "grammar.used must have 1 to 200 characters"},
		{name: "201 letters in right", change: func(c *CardCreate) { c.Grammar.Right = strings.Repeat("ż", 201) }, want: "grammar.right must have 1 to 200 characters"},
		{name: "right equal to used with other capitals and spaces", change: func(c *CardCreate) { c.Grammar.Right = "Dwa   PIESY" },
			want: "grammar.right must be different from grammar.used"},
		{name: "201 letters in hint", change: func(c *CardCreate) { c.Grammar.Hint = strings.Repeat("ż", 201) }, want: "grammar.hint must have 1 to 200 characters"},
		{name: "a hint with used and more words", change: func(c *CardCreate) {
			c.Grammar.Used, c.Grammar.Right, c.Grammar.Hint = "kupuję", "kupiłem", "nie kupuję, tylko czas przeszły"
		}, want: "grammar.hint must not contain grammar.used"},
		{name: "used at the end of the hint", change: func(c *CardCreate) {
			c.Grammar.Used, c.Grammar.Right, c.Grammar.Hint = "kupuję", "kupiłem", "czas przeszły, nie kupuję"
		}, want: "grammar.hint must not contain grammar.used"},
		{name: "the hint rule ignores capitals", change: func(c *CardCreate) {
			c.Grammar.Used, c.Grammar.Right, c.Grammar.Hint = "kupuję", "kupiłem", "Nie KUPUJĘ, tylko czas przeszły"
		}, want: "grammar.hint must not contain grammar.used"},
		// The hint has e and U+0328 COMBINING OGONEK in place of ę. The mark is not a letter, so only the NFC form of the hint
		// has the word kupuję.
		{name: "a decomposed letter in the hint", change: func(c *CardCreate) {
			c.Grammar.Used, c.Grammar.Right, c.Grammar.Hint = "kupuję", "kupiłem", "nie kupuje\u0328, tylko czas przeszły"
		}, want: "grammar.hint must not contain grammar.used"},
		{name: "five facts", change: func(c *CardCreate) { c.Grammar.Facts = []string{"a", "b", "c", "d", "e"} }, want: "grammar.facts must have at most 4 labels of 1 to 40 characters"},
		{name: "a fact of 41 letters", change: func(c *CardCreate) { c.Grammar.Facts[0] = strings.Repeat("ż", 41) }, want: "grammar.facts must have at most 4 labels of 1 to 40 characters"},
		{name: "six slots", change: func(c *CardCreate) { c.Grammar.Slots = []string{"a", "b", "c", "d", "e", "f"} }, want: "grammar.slots must have at most 5 labels of 1 to 40 characters"},
		{name: "a slot of 41 letters", change: func(c *CardCreate) { c.Grammar.Slots[0] = strings.Repeat("ż", 41) }, want: "grammar.slots must have at most 5 labels of 1 to 40 characters"},
		{name: "501 letters in rule", change: func(c *CardCreate) { c.Grammar.Rule = strings.Repeat("ż", 501) }, want: "grammar.rule must have 1 to 500 characters"},
		{name: "a rule with a line break", change: func(c *CardCreate) {
			c.Grammar.Rule = "W liczbie mnogiej „pies” traci „e”: psy.\nTak samo: sen, sny."
		}, want: "grammar.rule must have 1 to 500 characters"},
		{name: "one example", change: func(c *CardCreate) { c.Grammar.Examples = []string{"pies → psy"} }, want: "grammar.examples must have 2 or 3 texts of 1 to 200 characters"},
		{name: "four examples", change: func(c *CardCreate) { c.Grammar.Examples = []string{"a", "b", "c", "d"} }, want: "grammar.examples must have 2 or 3 texts of 1 to 200 characters"},
		{name: "an example of 201 letters", change: func(c *CardCreate) { c.Grammar.Examples[0] = strings.Repeat("ż", 201) },
			want: "grammar.examples must have 2 or 3 texts of 1 to 200 characters"},
		{name: "101 letters in the table title", change: func(c *CardCreate) { c.Grammar.Table.Title = strings.Repeat("ż", 101) },
			want: "grammar.table.title must have 1 to 100 characters"},
		{name: "two columns", change: func(c *CardCreate) { c.Grammar.Table.Columns = []string{"Rodzaj", "Końcówka"} },
			want: "grammar.table.columns must have 3 texts of 1 to 40 characters"},
		{name: "four columns", change: func(c *CardCreate) { c.Grammar.Table.Columns = []string{"Rodzaj", "Końcówka", "Przykład", "Uwaga"} },
			want: "grammar.table.columns must have 3 texts of 1 to 40 characters"},
		{name: "a column of 41 letters", change: func(c *CardCreate) { c.Grammar.Table.Columns[0] = strings.Repeat("ż", 41) },
			want: "grammar.table.columns must have 3 texts of 1 to 40 characters"},
		{name: "no group", change: func(c *CardCreate) { c.Grammar.Table.Groups = []GrammarGroup{} }, want: "grammar.table.groups must have 1 to 4 groups"},
		{name: "five groups", change: func(c *CardCreate) {
			c.Grammar.Table.Groups = append(c.Grammar.Table.Groups, GrammarGroup{Rows: grammarRows(1)}, GrammarGroup{Rows: grammarRows(1)}, GrammarGroup{Rows: grammarRows(1)})
		}, want: "grammar.table.groups must have 1 to 4 groups"},
		{name: "a blank group title", change: func(c *CardCreate) { c.Grammar.Table.Groups[0].Title = new(" ") }, want: "grammar.table.groups.title must have 1 to 80 characters"},
		{name: "a group title of 81 letters", change: func(c *CardCreate) { c.Grammar.Table.Groups[0].Title = new(strings.Repeat("ż", 81)) },
			want: "grammar.table.groups.title must have 1 to 80 characters"},
		{name: "a group without rows", change: func(c *CardCreate) { c.Grammar.Table.Groups[0].Rows = []GrammarRow{} },
			want: "grammar.table.groups.rows must have 1 to 8 rows in each group"},
		{name: "nine rows in one group", change: func(c *CardCreate) { c.Grammar.Table.Groups[0].Rows = grammarRows(9) },
			want: "grammar.table.groups.rows must have 1 to 8 rows in each group"},
		{name: "a row without names", change: func(c *CardCreate) {
			r := &c.Grammar.Table.Groups[1].Rows[0]
			r.Names, r.Examples = []string{}, []string{}
		}, want: "grammar.table.groups.rows.names must have 1 to 3 texts of 1 to 40 characters"},
		{name: "a row with four names", change: func(c *CardCreate) {
			r := &c.Grammar.Table.Groups[1].Rows[0]
			r.Names, r.Examples = append(r.Names, "męski"), append(r.Examples, "x")
		}, want: "grammar.table.groups.rows.names must have 1 to 3 texts of 1 to 40 characters"},
		{name: "a name of 41 letters", change: func(c *CardCreate) { c.Grammar.Table.Groups[1].Rows[0].Names[0] = strings.Repeat("ż", 41) },
			want: "grammar.table.groups.rows.names must have 1 to 3 texts of 1 to 40 characters"},
		{name: "endings of 61 letters", change: func(c *CardCreate) { c.Grammar.Table.Groups[1].Rows[0].Endings = strings.Repeat("ż", 61) },
			want: "grammar.table.groups.rows.endings must have 1 to 60 characters"},
		{name: "fewer examples than names", change: func(c *CardCreate) { c.Grammar.Table.Groups[1].Rows[0].Examples = []string{"pies → psy"} },
			want: "grammar.table.groups.rows.examples must have one text of 1 to 80 characters for each name"},
		{name: "a row example of 81 letters", change: func(c *CardCreate) { c.Grammar.Table.Groups[1].Rows[0].Examples[0] = strings.Repeat("ż", 81) },
			want: "grammar.table.groups.rows.examples must have one text of 1 to 80 characters for each name"},
		{name: "thirteen rows", change: func(c *CardCreate) {
			table := c.Grammar.Table
			table.Groups[0].Rows = grammarRows(8)
			table.Groups = append(table.Groups, GrammarGroup{Rows: grammarRows(3)})
		}, want: "grammar.table must have at most 12 rows"},
		{name: "no hit row", change: func(c *CardCreate) { c.Grammar.Table.Groups[1].Rows[0].Hit = false }, want: "grammar.table must have exactly one row with hit true"},
		{name: "two hit rows", change: func(c *CardCreate) { c.Grammar.Table.Groups[0].Rows[0].Hit = true }, want: "grammar.table must have exactly one row with hit true"},
		{name: "the first broken rule gives the message", change: func(c *CardCreate) { c.Pl, c.Grammar = "", nil }, want: "pl must have 1 to 200 characters"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := grammarCreate(tt.change)
			if got := CheckGrammarCard(&c); got != tt.want {
				t.Errorf("CheckGrammarCard() = %q, want %q", got, tt.want)
			}
			if tt.wantCard == nil {
				return
			}
			// reflect.DeepEqual treats a nil list and an empty list as different. The JSON shows a nil list as null. It
			// leaves out alternativesPl and alternativesRu when they are nil or empty, because their fields have omitempty.
			if want := grammarCreate(tt.wantCard); !reflect.DeepEqual(c, want) {
				gotJSON, _ := json.Marshal(c, json.FormatNilSliceAsNull(true))
				wantJSON, _ := json.Marshal(want, json.FormatNilSliceAsNull(true))
				t.Errorf("card = %s, want %s", gotJSON, wantJSON)
			}
		})
	}
}
