package validator

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"

	"s1_workflow_validator_api/models"

	"system1/scenario/internal/workflowhttp"
)

type validationError string

func (err validationError) Error() string { return string(err) }

func blockResponse(c echo.Context, err error, status int) error {
	var invalid validationError
	if errors.As(err, &invalid) {
		return c.JSON(http.StatusUnprocessableEntity, models.ValidationErrorDto{
			Error: "VALIDATION_ERROR", Message: invalid.Error(), Metadata: "{}",
		})
	}
	if err != nil {
		return err
	}
	return c.NoContent(status)
}

func (h *handler) GetWorkflowBlockMetadata(c echo.Context) error {
	return c.JSON(http.StatusOK, h.catalog.dto)
}

func (h *handler) GetWorkflowBlock(c echo.Context) error {
	workflowID, blockID, err := blockIdentifiers(c)
	if err != nil {
		return err
	}
	var block models.WorkflowBlockDto
	err = h.db.QueryRow(c.Request().Context(), `
		SELECT b.id, b.type, b.metadata::text FROM workflow_validator.blocks b
		JOIN workflow_validator.workflows w ON w.id = b.workflow_id
		JOIN owners.services s ON s.id = w.service_id
		WHERE w.id = $1 AND b.id = $2 AND s.owner_id = $3 AND NOT s.removed`,
		workflowID, blockID, workflowhttp.TemporaryUserID(c.Request().Context())).Scan(&block.Id, &block.Type, &block.Metadata)
	if errors.Is(err, pgx.ErrNoRows) {
		return echo.NewHTTPError(http.StatusNotFound, "Block not found")
	}
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, block)
}

func (h *handler) ListWorkflowBlocks(c echo.Context) error {
	workflowID, err := workflowhttp.UUID(c.Param("workflowId"), "workflowId")
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	var result models.WorkflowBlockListDto
	if err := h.withWorkflow(ctx, workflowID, func(tx pgx.Tx, serviceID string) error {
		var err error
		result.ServiceId = serviceID
		result.Blocks, err = listBlocks(ctx, tx, workflowID)
		return err
	}); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, result)
}

func (h *handler) UpsertWorkflowBlockBatch(c echo.Context) error {
	workflowID, err := workflowhttp.UUID(c.Param("workflowId"), "workflowId")
	if err != nil {
		return err
	}
	var body models.UpsertWorkflowBlocksRequest
	if err := workflowhttp.ReadJSON(c, &body); err != nil {
		return err
	}
	err = h.saveBlocks(c.Request().Context(), workflowID, body.Blocks, false)
	return blockResponse(c, err, http.StatusNoContent)
}

func (h *handler) UpdateWorkflowBlock(c echo.Context) error {
	workflowID, blockID, err := blockIdentifiers(c)
	if err != nil {
		return err
	}
	var body models.UpdateWorkflowBlockRequest
	if err := workflowhttp.ReadJSON(c, &body); err != nil {
		return err
	}
	block := models.WorkflowBlockDto{Id: blockID, Type: body.Type, Metadata: body.Metadata}
	if err := h.saveBlocks(c.Request().Context(), workflowID, []models.WorkflowBlockDto{block}, true); err != nil {
		return blockResponse(c, err, http.StatusOK)
	}
	return c.JSON(http.StatusOK, block)
}

func (h *handler) DeleteWorkflowBlock(c echo.Context) error {
	workflowID, blockID, err := blockIdentifiers(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	err = h.withWorkflow(ctx, workflowID, func(tx pgx.Tx, _ string) error {
		tag, err := tx.Exec(ctx, "DELETE FROM workflow_validator.blocks WHERE workflow_id = $1 AND id = $2", workflowID, blockID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return echo.NewHTTPError(http.StatusNotFound, "Block not found")
		}
		blocks, err := listBlocks(ctx, tx, workflowID)
		if err != nil {
			return err
		}
		for _, block := range blocks {
			targets, err := transitions(block.Metadata)
			if err != nil {
				return err
			}
			for _, target := range targets {
				if target == blockID {
					return validationError(fmt.Sprintf("Block %q references %q", block.Id, blockID))
				}
			}
		}
		return nil
	})
	return blockResponse(c, err, http.StatusOK)
}

func (h *handler) saveBlocks(ctx context.Context, workflowID string, blocks []models.WorkflowBlockDto, replace bool) error {
	if len(blocks) == 0 {
		return validationError("At least one block is required")
	}
	for _, block := range blocks {
		if err := h.catalog.validate(block); err != nil {
			return validationError(err.Error())
		}
	}

	return h.withWorkflow(ctx, workflowID, func(tx pgx.Tx, _ string) error {
		rows, err := tx.Query(ctx, "SELECT id FROM workflow_validator.blocks WHERE workflow_id = $1", workflowID)
		if err != nil {
			return err
		}
		persisted, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		ids := make(map[string]bool, len(persisted)+len(blocks))
		for _, id := range persisted {
			ids[id] = false
		}
		for _, block := range blocks {
			inBatch, exists := ids[block.Id]
			if replace && !exists {
				return echo.NewHTTPError(http.StatusNotFound, "Block not found")
			}
			if inBatch {
				return validationError(fmt.Sprintf("Duplicate block id %q", block.Id))
			}
			ids[block.Id] = true
		}
		for _, block := range blocks {
			targets, err := transitions(block.Metadata)
			if err != nil {
				return err
			}
			for _, target := range targets {
				if _, exists := ids[target]; target != "" && !exists {
					return validationError(fmt.Sprintf("Block %q references missing block %q", block.Id, target))
				}
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO workflow_validator.blocks (workflow_id, id, type, metadata)
				VALUES ($1, $2, $3, $4::jsonb)
				ON CONFLICT (workflow_id, id)
				DO UPDATE SET type = EXCLUDED.type, metadata = EXCLUDED.metadata`,
				workflowID, block.Id, block.Type, block.Metadata); err != nil {
				return err
			}
		}
		return nil
	})
}

func listBlocks(ctx context.Context, tx pgx.Tx, workflowID string) ([]models.WorkflowBlockDto, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, type, metadata::text FROM workflow_validator.blocks
		WHERE workflow_id = $1 ORDER BY id`, workflowID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[models.WorkflowBlockDto])
}

func blockIdentifiers(c echo.Context) (string, string, error) {
	workflowID, err := workflowhttp.UUID(c.Param("workflowId"), "workflowId")
	blockID := c.QueryParam("blockId")
	if err == nil && blockID == "" {
		err = echo.NewHTTPError(http.StatusBadRequest, "blockId is required")
	}
	return workflowID, blockID, err
}
