package api

import (
	"context"
	"encoding/json/v2"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestCreateCard(t *testing.T) {
	const (
		baseBody      = `{"pl":"dom","ru":"дом","examplePl":"To jest dom.","exampleRu":"Это дом.","meaningRu":"здание"}`
		plRule        = `{"message":"pl must have 1 to 200 characters"}`
		ruRule        = `{"message":"ru must have 1 to 200 characters"}`
		examplePlRule = `{"message":"examplePl must have 1 to 500 characters"}`
		exampleRuRule = `{"message":"exampleRu must have 1 to 500 characters"}`
		meaningRuRule = `{"message":"meaningRu must have 1 to 200 characters"}`
		altPlRule     = `{"message":"alternativesPl must have at most 8 terms of 1 to 200 characters"}`
		altRuRule     = `{"message":"alternativesRu must have at most 8 terms of 1 to 200 characters"}`
	)
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
			// reflect.DeepEqual tells a nil list from an empty one.
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
		setup      string // one statement that adds cards to the fixture; it finds a group by its name
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
		{name: "cards with one term are in the order of their IDs", sub: subOla, wantStatus: 200,
			setup: `INSERT INTO cards (group_id, id, pl, ru, example_pl, example_ru, meaning_ru)
				SELECT (SELECT id FROM groups WHERE name = 'Rodzina'), v.id, 'dom', 'дом', 'To jest dom.', 'Это дом.', 'здание'
				FROM (VALUES ('00000000-0000-0000-0000-000000000002'::uuid), ('00000000-0000-0000-0000-000000000001'::uuid)) AS v (id)`,
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
				// reflect.DeepEqual tells a nil list from an empty one.
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
