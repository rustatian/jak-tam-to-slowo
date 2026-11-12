// Command admin is the tool of the operator. Its command new-group sends a new group invitation to an email
// address: the person signs up with the link and starts a group.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"

	"github.com/rustatian/jak-tam-to-slowo/backend/internal/api"
	"github.com/rustatian/jak-tam-to-slowo/backend/internal/db"
	"github.com/rustatian/jak-tam-to-slowo/backend/internal/sesmail"
)

const usage = "usage: admin new-group -endpoint <ClusterEndpoint> -stage <dev|prod> <email>"

func main() {
	if len(os.Args) < 2 || os.Args[1] != "new-group" {
		log.Fatal(usage)
	}
	flags := flag.NewFlagSet("new-group", flag.ContinueOnError)
	endpoint := flags.String("endpoint", "", "DSQL cluster endpoint (stack output ClusterEndpoint)")
	region := flags.String("region", "eu-central-1", "region of the cluster and of Amazon SES")
	stage := flags.String("stage", "", "dev or prod; the emails go out with the SES configuration set slowo-<stage>")
	if err := flags.Parse(os.Args[2:]); err != nil {
		log.Fatal(err)
	}
	if *endpoint == "" || (*stage != "dev" && *stage != "prod") || flags.NArg() != 1 {
		log.Fatal(usage)
	}

	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(*region))
	if err != nil {
		log.Fatal(err)
	}
	// The operator connects as the admin role, like the migration command.
	pool, err := db.Open(ctx, db.Config{Endpoint: *endpoint, Region: *region, User: "admin", Credentials: cfg.Credentials})
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	srv := &api.Server{DB: pool, Send: sesmail.Sender(sesv2.NewFromConfig(cfg), "noreply@slowo.rustatian.me", "slowo-"+*stage)}
	if err := srv.InviteToNewGroup(ctx, flags.Arg(0)); err != nil {
		log.Fatal(err)
	}
	fmt.Println("invitation sent")
}
