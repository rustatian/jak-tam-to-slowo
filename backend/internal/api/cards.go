package api

import (
	"cmp"
	"context"
	"encoding/json/v2"
	"errors"
	"net/http"
	"slices"

	"github.com/aws/aws-lambda-go/events"
	"github.com/jackc/pgx/v5"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"

	"github.com/rustatian/jak-tam-to-slowo/backend/internal/db"
)

// cardColumns are the columns of a card in the order of scanCard.
const cardColumns = "id, kind, pl, ru, example_pl, example_ru, meaning_ru, alternatives_pl, alternatives_ru"

func scanCard(row pgx.Row) (Card, error) {
	var c Card
	err := row.Scan(&c.Id, &c.Kind, &c.Pl, &c.Ru, &c.ExamplePl, &c.ExampleRu, &c.MeaningRu, &c.AlternativesPl, &c.AlternativesRu)
	return c, err
}

// createCard adds a translation card to the group in the path.
func (s *Server) createCard(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	sub, fail := s.member(ctx, req)
	if fail != nil {
		return *fail
	}
	var body CardCreate
	if err := json.Unmarshal([]byte(req.Body), &body); err != nil {
		return jsonResponse(http.StatusBadRequest, Error{Message: "invalid JSON body"})
	}
	// A list that the body does not have is stored as [].
	in := CardUpdate{Pl: &body.Pl, Ru: &body.Ru, ExamplePl: &body.ExamplePl, ExampleRu: &body.ExampleRu, MeaningRu: &body.MeaningRu,
		AlternativesPl: cmp.Or(body.AlternativesPl, &[]string{}), AlternativesRu: cmp.Or(body.AlternativesRu, &[]string{})}
	if msg := checkCard(&in); msg != "" {
		return jsonResponse(http.StatusBadRequest, Error{Message: msg})
	}
	var card Card
	err := db.Tx(ctx, s.DB, func(tx pgx.Tx) error {
		var err error
		card, err = scanCard(tx.QueryRow(ctx,
			"INSERT INTO cards (group_id, pl, ru, example_pl, example_ru, meaning_ru, alternatives_pl, alternatives_ru) VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING "+cardColumns,
			req.PathParameters["groupId"], in.Pl, in.Ru, in.ExamplePl, in.ExampleRu, in.MeaningRu, in.AlternativesPl, in.AlternativesRu))
		return err
	})
	if err != nil {
		return serverError(ctx, req, sub, err)
	}
	return jsonResponse(http.StatusCreated, card)
}

// listCards answers the cards of the group in the path that are not archived, in the Polish order of pl, then by ID.
func (s *Server) listCards(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	sub, fail := s.member(ctx, req)
	if fail != nil {
		return *fail
	}
	rows, err := s.DB.Query(ctx, "SELECT "+cardColumns+" FROM cards WHERE group_id = $1 AND NOT archived", req.PathParameters["groupId"])
	if err != nil {
		return serverError(ctx, req, sub, err)
	}
	cards, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Card, error) { return scanCard(row) })
	if err != nil {
		return serverError(ctx, req, sub, err)
	}
	// Aurora DSQL sorts text only by bytes, so the order is made here with a new Collator, as in listMembers.
	// https://docs.aws.amazon.com/aurora-dsql/latest/userguide/working-with-postgresql-compatibility-migration-guide.html#working-with-postgresql-compatibility-unsupported-limitations
	polish := collate.New(language.Polish)
	slices.SortFunc(cards, func(a, b Card) int {
		return cmp.Or(polish.CompareString(a.Pl, b.Pl), cmp.Compare(a.Id, b.Id))
	})
	return jsonResponse(http.StatusOK, Cards{Cards: cards})
}

// patchCard changes the card fields that the body names. A field that the body leaves out keeps its value.
func (s *Server) patchCard(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	sub, fail := s.member(ctx, req)
	if fail != nil {
		return *fail
	}
	var update CardUpdate
	if err := json.Unmarshal([]byte(req.Body), &update); err != nil {
		return jsonResponse(http.StatusBadRequest, Error{Message: "invalid JSON body"})
	}
	if msg := checkCard(&update); msg != "" {
		return jsonResponse(http.StatusBadRequest, Error{Message: msg})
	}
	var card Card
	err := db.Tx(ctx, s.DB, func(tx pgx.Tx) error {
		var err error
		// The ID is compared as text: PostgreSQL refuses a uuid parameter that is not a UUID (SQLSTATE 22P02).
		// https://www.postgresql.org/docs/16/errcodes-appendix.html
		card, err = scanCard(tx.QueryRow(ctx,
			"UPDATE cards SET pl = COALESCE($3, pl), ru = COALESCE($4, ru), example_pl = COALESCE($5, example_pl), example_ru = COALESCE($6, example_ru), "+
				"meaning_ru = COALESCE($7, meaning_ru), alternatives_pl = COALESCE($8, alternatives_pl), alternatives_ru = COALESCE($9, alternatives_ru) "+
				"WHERE group_id = $1 AND id::text = $2 AND NOT archived RETURNING "+cardColumns,
			req.PathParameters["groupId"], req.PathParameters["cardId"],
			update.Pl, update.Ru, update.ExamplePl, update.ExampleRu, update.MeaningRu, update.AlternativesPl, update.AlternativesRu))
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return jsonResponse(http.StatusNotFound, Error{Message: "card not found"})
	}
	if err != nil {
		return serverError(ctx, req, sub, err)
	}
	return jsonResponse(http.StatusOK, card)
}

// checkCard trims each text and each term that u has, in place. msg is the 400 text of the first broken rule, in
// the order of the fields. It is empty when u passes. A field that u does not have stays nil.
func checkCard(u *CardUpdate) (msg string) {
	switch {
	case !checkText(u.Pl, 200):
		return "pl must have 1 to 200 characters"
	case !checkText(u.Ru, 200):
		return "ru must have 1 to 200 characters"
	case !checkText(u.ExamplePl, 500):
		return "examplePl must have 1 to 500 characters"
	case !checkText(u.ExampleRu, 500):
		return "exampleRu must have 1 to 500 characters"
	case !checkText(u.MeaningRu, 200):
		return "meaningRu must have 1 to 200 characters"
	case !checkTerms(u.AlternativesPl):
		return "alternativesPl must have at most 8 terms of 1 to 200 characters"
	case !checkTerms(u.AlternativesRu):
		return "alternativesRu must have at most 8 terms of 1 to 200 characters"
	}
	return ""
}

// checkText applies trimmedText to the text that p points to, in place. A nil p passes.
func checkText(p *string, max int) bool {
	if p == nil {
		return true
	}
	var ok bool
	*p, ok = trimmedText(*p, max)
	return ok
}

// checkTerms applies trimmedText with 200 characters to each term of the list that p points to, in place. It
// reports that the list has at most 8 terms and that each term passes. A nil p passes. The slice stays the same, so
// a decoded list stays not nil: pgx encodes jsonb with json.Marshal, which encodes a nil slice as the JSON value null.
// https://pkg.go.dev/encoding/json#Marshal
func checkTerms(p *[]string) bool {
	if p == nil {
		return true
	}
	if len(*p) > 8 {
		return false
	}
	for i, term := range *p {
		var ok bool
		if (*p)[i], ok = trimmedText(term, 200); !ok {
			return false
		}
	}
	return true
}
