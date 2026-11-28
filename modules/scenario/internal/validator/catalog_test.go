package validator

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"s1_workflow_validator_api/models"
)

func TestCatalogExamples(t *testing.T) {
	catalog, err := LoadCatalog(filepath.Join("..", "..", "..", "..", "api", "src", "workflow-blocks.schema.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.dto.BlockTypes) != 10 {
		t.Fatalf("got %d block types", len(catalog.dto.BlockTypes))
	}
	for _, entry := range catalog.dto.BlockTypes {
		t.Run(entry.Type, func(t *testing.T) {
			var schema struct {
				Examples []json.RawMessage `json:"examples"`
			}
			if err := json.Unmarshal([]byte(entry.Schema), &schema); err != nil || len(schema.Examples) == 0 {
				t.Fatalf("missing examples: %v", err)
			}
			for _, sample := range schema.Examples {
				if err := catalog.validate(models.WorkflowBlockDto{Id: "test", Type: entry.Type, Metadata: string(sample)}); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
