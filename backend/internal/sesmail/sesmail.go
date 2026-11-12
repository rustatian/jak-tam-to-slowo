// Package sesmail sends the emails of the app through Amazon SES.
package sesmail

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"

	"github.com/rustatian/jak-tam-to-slowo/backend/internal/api"
)

// Sender returns the send function for api.Server. The configuration set of the stage publishes the bounces and
// the complaints of the emails.
func Sender(client *sesv2.Client, from, configurationSet string) func(context.Context, api.Mail) error {
	// The emails have letters outside ASCII, and SES uses 7-bit ASCII when the content names no character set.
	// https://docs.aws.amazon.com/ses/latest/APIReference-V2/API_Content.html
	charset := "UTF-8"
	return func(ctx context.Context, m api.Mail) error {
		_, err := client.SendEmail(ctx, &sesv2.SendEmailInput{
			FromEmailAddress:     &from,
			ConfigurationSetName: &configurationSet,
			// A bounce or a complaint event names the invitation by this message tag.
			EmailTags:   []types.MessageTag{{Name: aws.String("invitation"), Value: &m.Invitation}},
			Destination: &types.Destination{ToAddresses: []string{m.To}},
			Content: &types.EmailContent{Simple: &types.Message{
				Subject: &types.Content{Data: &m.Subject, Charset: &charset},
				Body:    &types.Body{Text: &types.Content{Data: &m.Text, Charset: &charset}},
			}},
		})
		return err
	}
}
