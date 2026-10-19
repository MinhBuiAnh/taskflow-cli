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
)

func main() {
	ctx := context.Background()
	dsn := "taskflow.db"
	if len(os.Args) > 1 {
		dsn = os.Args[1]
	}
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
