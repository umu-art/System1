package main

import (
	"context"
	"log"

	"system1/scenario/internal/validator"
	"system1/scenario/internal/workflowhttp"
)

func main() {
	if err := start(); err != nil {
		log.Fatal(err)
	}
}

func start() error {
	path := workflowhttp.Env("WORKFLOW_CATALOG_PATH", "../../api/src/workflow-blocks.schema.yaml")
	catalog, err := validator.LoadCatalog(path)
	if err != nil {
		return err
	}
	db, err := workflowhttp.OpenDB(context.Background())
	if err != nil {
		return err
	}
	defer db.Close()

	return workflowhttp.Serve(validator.NewServer(db, catalog))
}
