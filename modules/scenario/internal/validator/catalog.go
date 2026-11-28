package validator

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"

	"s1_workflow_validator_api/models"
)

type Catalog struct {
	dto     models.WorkflowBlockCatalogDto
	schemas map[string]*jsonschema.Schema
}

func LoadCatalog(path string) (*Catalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var document struct {
		Components struct {
			Examples map[string]struct {
				Value struct {
					BlockTypes []struct {
						Type        string
						Description string
						Schema      string
					} `yaml:"blockTypes"`
				}
			}
		}
	}
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, err
	}
	entries := document.Components.Examples["workflow-block-catalog-example"].Value.BlockTypes
	if len(entries) == 0 {
		return nil, fmt.Errorf("catalog has no block types")
	}

	catalog := &Catalog{schemas: make(map[string]*jsonschema.Schema, len(entries))}
	for _, entry := range entries {
		if entry.Type == "" || catalog.schemas[entry.Type] != nil {
			return nil, fmt.Errorf("empty or duplicate catalog type %q", entry.Type)
		}
		compiler := jsonschema.NewCompiler()
		compiler.AssertFormat()
		schema, err := jsonschema.UnmarshalJSON(strings.NewReader(entry.Schema))
		if err != nil {
			return nil, fmt.Errorf("decode schema for %s: %w", entry.Type, err)
		}
		if err := compiler.AddResource("metadata.json", schema); err != nil {
			return nil, fmt.Errorf("load schema for %s: %w", entry.Type, err)
		}
		compiled, err := compiler.Compile("metadata.json")
		if err != nil {
			return nil, fmt.Errorf("compile schema for %s: %w", entry.Type, err)
		}

		catalog.schemas[entry.Type] = compiled
		catalog.dto.BlockTypes = append(catalog.dto.BlockTypes, models.WorkflowBlockTypeDto{
			Type: entry.Type, Description: entry.Description, Schema: entry.Schema,
		})
	}
	return catalog, nil
}

func (c *Catalog) validate(block models.WorkflowBlockDto) error {
	schema, ok := c.schemas[block.Type]
	switch {
	case block.Id == "":
		return fmt.Errorf("block id is required")
	case !ok:
		return fmt.Errorf("block %q has unknown type %q", block.Id, block.Type)
	}
	metadata, err := jsonschema.UnmarshalJSON(strings.NewReader(block.Metadata))
	if err != nil || !json.Valid([]byte(block.Metadata)) {
		return fmt.Errorf("block %q metadata is invalid JSON", block.Id)
	}
	if err := schema.Validate(metadata); err != nil {
		return fmt.Errorf("block %q metadata: %w", block.Id, err)
	}
	return nil
}

func transitions(metadata string) ([]string, error) {
	var fields struct {
		Next        string
		OnError     string
		Default     string
		OnSucceeded string
		OnCanceled  string
		Cases       []struct{ Next string }
	}
	if err := json.Unmarshal([]byte(metadata), &fields); err != nil {
		return nil, err
	}
	targets := []string{fields.Next, fields.OnError, fields.Default, fields.OnSucceeded, fields.OnCanceled}
	for _, candidate := range fields.Cases {
		targets = append(targets, candidate.Next)
	}
	return targets, nil
}
