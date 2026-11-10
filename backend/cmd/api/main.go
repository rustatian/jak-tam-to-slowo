// Command api is the API Lambda.
package main

import (
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/rustatian/jak-tam-to-slowo/backend/internal/api"
)

func main() {
	lambda.Start(api.Handle)
}
