// Command api is the API Lambda.
package main

import (
	"context"
	"errors"
	"log"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	sestypes "github.com/aws/aws-sdk-go-v2/service/sesv2/types"

	"github.com/rustatian/jak-tam-to-slowo/backend/internal/api"
	"github.com/rustatian/jak-tam-to-slowo/backend/internal/db"
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
	// The emails have letters outside ASCII, and SES uses 7-bit ASCII when the content names no character set.
	// https://docs.aws.amazon.com/ses/latest/APIReference-V2/API_Content.html
	charset := "UTF-8"
	srv := &api.Server{DB: pool, EmailOf: func(ctx context.Context, sub string) (string, error) {
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
	}, Send: func(ctx context.Context, m api.Mail) error {
		_, err := mailer.SendEmail(ctx, &sesv2.SendEmailInput{
			FromEmailAddress:     &from,
			ConfigurationSetName: &configurationSet,
			// A bounce or a complaint event names the invitation by this message tag.
			EmailTags:   []sestypes.MessageTag{{Name: aws.String("invitation"), Value: &m.Invitation}},
			Destination: &sestypes.Destination{ToAddresses: []string{m.To}},
			Content: &sestypes.EmailContent{Simple: &sestypes.Message{
				Subject: &sestypes.Content{Data: &m.Subject, Charset: &charset},
				Body:    &sestypes.Body{Text: &sestypes.Content{Data: &m.Text, Charset: &charset}},
			}},
		})
		return err
	}}
	lambda.Start(srv.Handle)
}
