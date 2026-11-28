package main

import (
	"context"
	"log"
	"os"

	"system1/scenario/internal/executor"
	"system1/scenario/internal/workflowhttp"
)

func main() {
	if err := start(); err != nil {
		log.Fatal(err)
	}
}

func start() error {
	db, err := workflowhttp.OpenDB(context.Background())
	if err != nil {
		return err
	}
	defer db.Close()

	server, err := executor.NewServer(db, os.Getenv("WORKFLOW_VALIDATOR_URL"), workflowhttp.Env("S1_ENV", "local"))
	if err != nil {
		return err
	}
	return workflowhttp.Serve(server)
}
