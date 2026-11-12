// Command migrate applies the SQL migrations to Aurora DSQL as the admin role, or to PostgreSQL.
// With -app-role-arn it also maps the database role app to the IAM role of a Lambda function. Run it one time for
// each function role of the stack.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rustatian/jak-tam-to-slowo/backend/internal/db"
	"github.com/rustatian/jak-tam-to-slowo/backend/migrations"
)

func main() {
	endpoint := flag.String("endpoint", "", "DSQL cluster endpoint (stack output ClusterEndpoint)")
	region := flag.String("region", "eu-central-1", "DSQL cluster region")
	url := flag.String("database-url", "", "PostgreSQL connection string, instead of -endpoint")
	roleARN := flag.String("app-role-arn", "", "IAM role of a Lambda function that uses the database (stack outputs ApiFunctionRoleArn and PreSignUpFunctionRoleArn, one run for each); DSQL only")
	flag.Parse()
	if (*endpoint == "") == (*url == "") {
		log.Fatal("give exactly one of -endpoint and -database-url")
	}
	if *roleARN != "" && *endpoint == "" {
		log.Fatal("-app-role-arn needs -endpoint")
	}

	ctx := context.Background()
	var pool *pgxpool.Pool
	var err error
	if *url != "" {
		pool, err = db.OpenURL(ctx, *url)
	} else {
		cfg, cfgErr := config.LoadDefaultConfig(ctx, config.WithRegion(*region))
		if cfgErr != nil {
			log.Fatal(cfgErr)
		}
		pool, err = db.Open(ctx, db.Config{Endpoint: *endpoint, Region: *region, User: "admin", Credentials: cfg.Credentials})
	}
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	applied, err := db.Migrate(ctx, pool, migrations.FS)
	for _, v := range applied {
		fmt.Println("applied", v)
	}
	if err != nil {
		log.Fatal(err)
	}
	if *roleARN != "" {
		if err := db.SetupAppRole(ctx, pool, *roleARN); err != nil {
			log.Fatal(err)
		}
		fmt.Println("role app ready")
	}
}
