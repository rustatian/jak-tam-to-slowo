package api

import (
	"context"
	"encoding/json/v2"
	"net/http"

	"github.com/aws/aws-lambda-go/events"
	"github.com/jackc/pgx/v5"

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
