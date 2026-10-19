/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/MinhBuiAnh/taskflow-cli/internal/cli"
	"github.com/MinhBuiAnh/taskflow-cli/internal/repository"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load(".env")
	
	ctx := context.Background()
	dsn := os.Getenv("DSN")
	if dsn == "" {
		log.Fatalf("Data source name is not properly set")
	}

	// Open the database and perform migrations
	db, err := repository.OpenSQLite(ctx, dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err := repository.Migrate(ctx, db); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("TaskFlow database ready: %s\n", dsn)
	
	cli.Execute()
}
