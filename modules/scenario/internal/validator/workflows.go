package validator

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"

	"s1_workflow_validator_api/handlers"
	"s1_workflow_validator_api/models"

	"system1/scenario/internal/workflowhttp"
)

type handler struct {
	db      *pgxpool.Pool
	catalog *Catalog
}

func NewServer(db *pgxpool.Pool, catalog *Catalog) *echo.Echo {
	e := workflowhttp.NewServer(db, `SELECT w.id, b.id, s.id
		FROM workflow_validator.workflows w
		CROSS JOIN workflow_validator.blocks b
		CROSS JOIN owners.services s WHERE false`)
	workflowhttp.SetTemporaryUserID(e, func(path string) bool {
		return path == "/v1/workflow" || strings.HasPrefix(path, "/v1/workflow/")
	})
	h := &handler{db: db, catalog: catalog}
	handlers.RouteWorkflowAPI(e, h)
	handlers.RouteBlockAPI(e, h)
	return e
}

func (h *handler) CreateWorkflow(c echo.Context) error {
	var body models.CreateWorkflowRequest
	if err := workflowhttp.ReadJSON(c, &body); err != nil {
		return err
	}
	serviceID, err := workflowhttp.UUID(body.ServiceId, "serviceId")
	if err != nil {
		return err
	}
	workflow, err := scanWorkflow(h.db.QueryRow(c.Request().Context(), `
		INSERT INTO workflow_validator.workflows (service_id)
		SELECT id FROM owners.services WHERE id = $1 AND owner_id = $2 AND NOT removed
		RETURNING id::text, service_id::text`, serviceID, workflowhttp.TemporaryUserID(c.Request().Context())))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, workflow)
}

func (h *handler) ListWorkflows(c echo.Context) error {
	rows, err := h.db.Query(c.Request().Context(), `
		SELECT w.id::text, w.service_id::text FROM workflow_validator.workflows w
		JOIN owners.services s ON s.id = w.service_id
		WHERE s.owner_id = $1 AND NOT s.removed ORDER BY w.created, w.id`, workflowhttp.TemporaryUserID(c.Request().Context()))
	if err != nil {
		return err
	}
	workflows, err := pgx.CollectRows(rows, pgx.RowToStructByPos[models.WorkflowDto])
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, models.WorkflowListDto{Workflows: workflows})
}

func (h *handler) GetWorkflow(c echo.Context) error {
	workflowID, err := workflowhttp.UUID(c.Param("workflowId"), "workflowId")
	if err != nil {
		return err
	}
	workflow, err := scanWorkflow(h.db.QueryRow(c.Request().Context(), `
		SELECT w.id::text, w.service_id::text FROM workflow_validator.workflows w
		JOIN owners.services s ON s.id = w.service_id
		WHERE w.id = $1 AND s.owner_id = $2 AND NOT s.removed`, workflowID, workflowhttp.TemporaryUserID(c.Request().Context())))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, workflow)
}

func (h *handler) UpdateWorkflow(c echo.Context) error {
	workflowID, err := workflowhttp.UUID(c.Param("workflowId"), "workflowId")
	if err != nil {
		return err
	}
	var body models.UpdateWorkflowRequest
	if err := workflowhttp.ReadJSON(c, &body); err != nil {
		return err
	}
	serviceID, err := workflowhttp.UUID(body.ServiceId, "serviceId")
	if err != nil {
		return err
	}
	workflow, err := scanWorkflow(h.db.QueryRow(c.Request().Context(), `
		UPDATE workflow_validator.workflows w SET service_id = $3, updated = now()
		WHERE w.id = $1
		  AND EXISTS (SELECT 1 FROM owners.services WHERE id = w.service_id AND owner_id = $2 AND NOT removed)
		  AND EXISTS (SELECT 1 FROM owners.services WHERE id = $3 AND owner_id = $2 AND NOT removed)
		RETURNING w.id::text, w.service_id::text`, workflowID, workflowhttp.TemporaryUserID(c.Request().Context()), serviceID))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, workflow)
}

func (h *handler) DeleteWorkflow(c echo.Context) error {
	workflowID, err := workflowhttp.UUID(c.QueryParam("workflowId"), "workflowId")
	if err != nil {
		return err
	}
	result, err := h.db.Exec(c.Request().Context(), `
		DELETE FROM workflow_validator.workflows w USING owners.services s
		WHERE w.id = $1 AND s.id = w.service_id AND s.owner_id = $2 AND NOT s.removed`, workflowID, workflowhttp.TemporaryUserID(c.Request().Context()))
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return echo.NewHTTPError(http.StatusNotFound)
	}
	return c.NoContent(http.StatusOK)
}

func scanWorkflow(row pgx.Row) (models.WorkflowDto, error) {
	var result models.WorkflowDto
	err := row.Scan(&result.WorkflowId, &result.ServiceId)
	if errors.Is(err, pgx.ErrNoRows) {
		err = echo.NewHTTPError(http.StatusNotFound, "Workflow or service not found")
	}
	return result, err
}

func (h *handler) withWorkflow(ctx context.Context, workflowID string, operation func(pgx.Tx, string) error) error {
	// Serialize the graph check and write with other edits to the same workflow.
	return pgx.BeginFunc(ctx, h.db, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			SELECT w.id::text, w.service_id::text FROM workflow_validator.workflows w
			JOIN owners.services s ON s.id = w.service_id
			WHERE w.id = $1 AND s.owner_id = $2 AND NOT s.removed FOR UPDATE OF w`, workflowID, workflowhttp.TemporaryUserID(ctx))
		workflow, err := scanWorkflow(row)
		if err != nil {
			return err
		}
		return operation(tx, workflow.ServiceId)
	})
}
