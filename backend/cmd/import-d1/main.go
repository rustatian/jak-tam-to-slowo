// Command import-d1 moves the cards and the progress rows of the old D1 export into a group of Aurora DSQL, as the
// admin role. Without -write the transaction rolls back and the command only prints the counts.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"

	"github.com/rustatian/jak-tam-to-slowo/backend/internal/d1import"
	"github.com/rustatian/jak-tam-to-slowo/backend/internal/db"
)

func main() {
	endpoint := flag.String("endpoint", "", "DSQL cluster endpoint (stack output ClusterEndpoint)")
	region := flag.String("region", "eu-central-1", "DSQL cluster region")
	dir := flag.String("dir", "", "folder of the export files cards.json and card_progress.json")
	group := flag.String("group", "", "ID of the target group")
	write := flag.Bool("write", false, "keep the rows; without it the transaction rolls back")
	users := map[string]string{}
	flag.Func("user", "<old user ID>=<new user ID>; give it one time for each person", func(v string) error {
		oldID, newID, ok := strings.Cut(v, "=")
		if !ok || oldID == "" || newID == "" {
			return fmt.Errorf("want <old user ID>=<new user ID>, got %q", v)
		}
		users[oldID] = newID
		return nil
	})
	flag.Parse()
	if *endpoint == "" || *dir == "" || *group == "" || len(users) == 0 {
		flag.Usage()
		os.Exit(2)
	}

	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(*region))
	if err != nil {
		log.Fatal(err)
	}
	pool, err := db.Open(ctx, db.Config{Endpoint: *endpoint, Region: *region, User: "admin", Credentials: cfg.Credentials})
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	rep, err := d1import.Run(ctx, pool, os.DirFS(*dir), d1import.Options{Group: *group, Users: users, Write: *write})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("translation cards written:", rep.Cards)
	fmt.Println("grammar cards left out:", rep.Grammar)
	oldIDs := make([]string, 0, len(rep.Progress))
	for id := range rep.Progress {
		oldIDs = append(oldIDs, id)
	}
	slices.Sort(oldIDs)
	for _, id := range oldIDs {
		fmt.Printf("progress rows written for %s: %d\n", id, rep.Progress[id])
	}
	if *write {
		fmt.Println("written")
	} else {
		fmt.Println("dry run: no row was written")
	}
}
