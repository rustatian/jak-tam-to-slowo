// Command presignup is the Lambda function that Amazon Cognito calls before each sign-up. It lets a sign-up pass
// only with the token of a pending invitation.
package main

import (
	"context"
	"log"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"

	"github.com/rustatian/jak-tam-to-slowo/backend/internal/api"
	"github.com/rustatian/jak-tam-to-slowo/backend/internal/db"
)

func main() {
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatal(err)
	}
	// cmd/migrate maps the database role app to the role of this function too.
	pool, err := db.Open(ctx, db.Config{Endpoint: os.Getenv("DSQL_ENDPOINT"), Region: cfg.Region, User: "app", Credentials: cfg.Credentials})
	if err != nil {
		log.Fatal(err)
	}
	lambda.Start((&api.Server{DB: pool}).PreSignUp)
}
