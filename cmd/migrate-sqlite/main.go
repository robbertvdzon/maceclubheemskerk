package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/robbertvdzon/maceclubheemskerk/internal/content"
)

func main() {
	if len(os.Args) != 2 || os.Getenv("DATABASE_URL") == "" {
		log.Fatal("usage: migrate-sqlite /data/maceclub.sqlite (DATABASE_URL is required)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	count, revision, err := content.MigrateSQLite(ctx, os.Args[1], os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal("SQLite migration failed")
	}
	fmt.Printf("migration complete: records=%d revision=%d\n", count, revision)
}
