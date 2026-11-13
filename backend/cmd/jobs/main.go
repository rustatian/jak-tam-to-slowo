// Command jobs is the Lambda function that runs the draft jobs. The API function starts it with one event for each
// stored job.
package main

import (
	"context"
	"log"
	"os"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"

	"github.com/rustatian/jak-tam-to-slowo/backend/internal/api"
	"github.com/rustatian/jak-tam-to-slowo/backend/internal/claude"
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
	// The key is a SecureString parameter, so the read needs decryption.
	param, err := ssm.NewFromConfig(cfg).GetParameter(ctx, &ssm.GetParameterInput{Name: aws.String(os.Getenv("ANTHROPIC_KEY_PARAMETER")), WithDecryption: aws.Bool(true)})
	if err != nil {
		log.Fatal(err)
	}
	client := anthropic.NewClient(option.WithAPIKey(aws.ToString(param.Parameter.Value)))
	lambda.Start((&api.Server{DB: pool, AI: claude.Caller(client)}).RunJob)
}
