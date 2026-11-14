// Command api is the API Lambda.
package main

import (
	"context"
	"encoding/json/v2"
	"errors"
	"log"
	"os"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/ssm"

	"github.com/rustatian/jak-tam-to-slowo/backend/internal/api"
	"github.com/rustatian/jak-tam-to-slowo/backend/internal/claude"
	"github.com/rustatian/jak-tam-to-slowo/backend/internal/db"
	"github.com/rustatian/jak-tam-to-slowo/backend/internal/sesmail"
)

func main() {
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatal(err)
	}
	pool, err := db.Open(ctx, db.Config{Endpoint: os.Getenv("DSQL_ENDPOINT"), Region: cfg.Region, User: "app", Credentials: cfg.Credentials})
	if err != nil {
		log.Fatal(err)
	}
	users := cognitoidentityprovider.NewFromConfig(cfg)
	poolID := os.Getenv("USER_POOL_ID")
	mailer := sesv2.NewFromConfig(cfg)
	from := os.Getenv("MAIL_FROM")
	// The configuration set of the stage publishes the bounces and the complaints of the emails.
	configurationSet := os.Getenv("MAIL_CONFIGURATION_SET")
	functions := awslambda.NewFromConfig(cfg)
	jobsFunction := os.Getenv("JOBS_FUNCTION")
	// The key is a SecureString parameter, so the read needs decryption.
	param, err := ssm.NewFromConfig(cfg).GetParameter(ctx, &ssm.GetParameterInput{Name: aws.String(os.Getenv("ANTHROPIC_KEY_PARAMETER")), WithDecryption: aws.Bool(true)})
	if err != nil {
		log.Fatal(err)
	}
	client := anthropic.NewClient(option.WithAPIKey(aws.ToString(param.Parameter.Value)))
	srv := &api.Server{DB: pool, AI: claude.Caller(client), EmailOf: func(ctx context.Context, sub string) (string, error) {
		// The pool uses email as the username attribute, so AdminGetUser takes the sub as the username.
		out, err := users.AdminGetUser(ctx, &cognitoidentityprovider.AdminGetUserInput{UserPoolId: &poolID, Username: &sub})
		if err != nil {
			return "", err
		}
		for _, a := range out.UserAttributes {
			if aws.ToString(a.Name) == "email" {
				return aws.ToString(a.Value), nil
			}
		}
		return "", errors.New("the user has no email attribute")
	}, Send: sesmail.Sender(mailer, from, configurationSet), StartJob: func(ctx context.Context, userID, jobID string) error {
		payload, err := json.Marshal(api.JobEvent{UserID: userID, JobID: jobID})
		if err != nil {
			return err
		}
		_, err = functions.Invoke(ctx, &awslambda.InvokeInput{FunctionName: &jobsFunction, InvocationType: types.InvocationTypeEvent, Payload: payload})
		return err
	}}
	lambda.Start(srv.Handle)
}
