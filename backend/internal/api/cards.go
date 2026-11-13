package api

import (
	"cmp"
	"context"
	"encoding/json/v2"
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
	in, msg := checkCard(body)
	if msg != "" {
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

// checkCard returns the trimmed texts and terms of the body in a card without ID and kind. msg is the 400 text of
// the first broken rule, in the order of the fields. It is empty when the body passes.
func checkCard(body CardCreate) (card Card, msg string) {
	var ok bool
	if card.Pl, ok = trimmedText(body.Pl, 200); !ok {
		return Card{}, "pl must have 1 to 200 characters"
	}
	if card.Ru, ok = trimmedText(body.Ru, 200); !ok {
		return Card{}, "ru must have 1 to 200 characters"
	}
	if card.ExamplePl, ok = trimmedText(body.ExamplePl, 500); !ok {
		return Card{}, "examplePl must have 1 to 500 characters"
	}
	if card.ExampleRu, ok = trimmedText(body.ExampleRu, 500); !ok {
		return Card{}, "exampleRu must have 1 to 500 characters"
	}
	if card.MeaningRu, ok = trimmedText(body.MeaningRu, 200); !ok {
		return Card{}, "meaningRu must have 1 to 200 characters"
	}
	if card.AlternativesPl, ok = trimmedTerms(body.AlternativesPl); !ok {
		return Card{}, "alternativesPl must have at most 8 terms of 1 to 200 characters"
	}
	if card.AlternativesRu, ok = trimmedTerms(body.AlternativesRu); !ok {
		return Card{}, "alternativesRu must have at most 8 terms of 1 to 200 characters"
	}
	return card, ""
}

// trimmedTerms applies trimmedText with 200 characters to each term of a list of at most 8 terms. A missing list
// is an empty list. The result is not nil: pgx sends a nil slice as SQL NULL, and the list columns are NOT NULL.
// https://pkg.go.dev/github.com/jackc/pgx/v5/pgtype#hdr-Encoding_Typed_Nils
func trimmedTerms(terms *[]string) (trimmed []string, ok bool) {
	trimmed = []string{}
	if terms == nil {
		return trimmed, true
	}
	if len(*terms) > 8 {
		return nil, false
	}
	for _, term := range *terms {
		if term, ok = trimmedText(term, 200); !ok {
			return nil, false
		}
		trimmed = append(trimmed, term)
	}
	return trimmed, true
}
