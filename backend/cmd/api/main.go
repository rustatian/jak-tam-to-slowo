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
	}}
	lambda.Start(srv.Handle)
}
