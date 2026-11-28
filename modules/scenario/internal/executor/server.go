package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/umu-art/System1/api/build/s1-workflow-validator-client"

	"s1_workflow_api/handlers"
	"s1_workflow_api/models"

	"system1/scenario/internal/workflowhttp"
)

type handler struct {
	db        *pgxpool.Pool
	validator *workflowvalidator.APIClient
	env       string
}

func NewServer(db *pgxpool.Pool, validatorURL, env string) (*echo.Echo, error) {
	upstream, err := url.Parse(validatorURL)
	if err != nil || upstream.Host == "" || (upstream.Scheme != "http" && upstream.Scheme != "https") ||
		upstream.User != nil || upstream.RawQuery != "" || upstream.Fragment != "" {
		return nil, errors.New("WORKFLOW_VALIDATOR_URL must be an HTTP(S) base URL")
	}
	config := workflowvalidator.NewConfiguration()
	config.Servers = workflowvalidator.ServerConfigurations{{URL: strings.TrimRight(validatorURL, "/")}}
	config.HTTPClient = &http.Client{Timeout: 5 * time.Second}

	e := workflowhttp.NewServer(db, `SELECT r.current_block_id, r.status, r.variables, r.state, s.owner_id, s.removed
		FROM workflow.runs r JOIN owners.services s ON s.id = r.service_id WHERE false`)
	workflowhttp.SetTemporaryUserID(e, func(path string) bool {
		return path == "/api/workflow/runs" || strings.HasPrefix(path, "/api/workflow/runs/")
	})
	handlers.RouteWorkflowAPI(e, &handler{db: db, validator: workflowvalidator.NewAPIClient(config), env: env})
	return e, nil
}

func (h *handler) GetWorkflowInfo(c echo.Context) error {
	return c.JSON(http.StatusOK, models.ServiceInfoDto{Service: "s1-workflow", Env: h.env})
}

func (h *handler) ValidateScenario(c echo.Context) error {
	return echo.NewHTTPError(http.StatusNotImplemented, "Use the workflow validator API")
}

func (h *handler) StartWorkflowRun(c echo.Context) error {
	var body models.StartWorkflowRunRequest
	if err := workflowhttp.ReadJSON(c, &body); err != nil {
		return err
	}
	runID, err := workflowhttp.UUID(body.RunId, "runId")
	if err != nil {
		return err
	}
	workflowID, err := workflowhttp.UUID(body.WorkflowId, "workflowId")
	if err != nil {
		return err
	}
	if body.EntryBlockId == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "entryBlockId is required")
	}

	ctx := c.Request().Context()
	current, err := loadRun(ctx, h.db, runID, false)
	if errors.Is(err, pgx.ErrNoRows) {
		current, err = h.start(ctx, runID, workflowID, body.EntryBlockId)
	}
	if err != nil {
		return runError(err)
	}
	if current.WorkflowID != workflowID || current.EntryBlock != body.EntryBlockId {
		return echo.NewHTTPError(http.StatusConflict, "runId is already used with different start parameters")
	}
	return c.JSON(http.StatusOK, current.view())
}

func (h *handler) start(ctx context.Context, runID, workflowID, entry string) (*run, error) {
	snapshot, serviceID, err := h.fetchSnapshot(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	first, exists := snapshot[entry]
	if !exists {
		return nil, echo.NewHTTPError(http.StatusNotFound, "Entry block not found")
	}
	var params parameters
	if err := json.Unmarshal(first.Metadata, &params); err != nil {
		return nil, echo.NewHTTPError(http.StatusBadGateway).SetInternal(err)
	}
	if first.Type != "wait_message" && (first.Type != "command" || params.Scope != "entry") {
		return nil, echo.NewHTTPError(http.StatusUnprocessableEntity, "Entry block must be wait_message or an entry command")
	}

	current := newRun(runID, workflowID, entry, snapshot)
	current.advance("")
	tag, err := h.db.Exec(ctx, `
		INSERT INTO workflow.runs (id, workflow_id, entry_block_id, service_id, snapshot, current_block_id, status, variables, state)
		SELECT $1, $2, $3, s.id, $5, NULLIF($6, ''), $7, $8, $9
		FROM owners.services s WHERE s.id = $4 AND s.owner_id = $10 AND NOT s.removed
		ON CONFLICT (id) DO NOTHING`,
		current.ID, current.WorkflowID, current.EntryBlock, serviceID, current.Graph,
		current.BlockID, current.Status, current.Variables, current.Data, workflowhttp.TemporaryUserID(ctx))
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		// A retry can find a concurrent start only while the caller owns its service.
		return loadRun(ctx, h.db, runID, false)
	}
	return current, nil
}

func (h *handler) fetchSnapshot(ctx context.Context, workflowID string) (graph, string, error) {
	data, response, err := h.validator.BlockAPI.ListWorkflowBlocks(ctx, workflowID).UserId(workflowhttp.TemporaryUserID(ctx)).Execute()
	if err != nil {
		if response != nil && response.StatusCode == http.StatusNotFound {
			return nil, "", echo.NewHTTPError(http.StatusNotFound, "Workflow not found")
		}
		// The generated client can include upstream response bodies in errors.
		return nil, "", echo.NewHTTPError(http.StatusBadGateway, "Validator could not return the workflow")
	}
	serviceID, err := workflowhttp.UUID(data.ServiceId, "serviceId")
	if err != nil {
		return nil, "", echo.NewHTTPError(http.StatusBadGateway, "Validator returned an invalid serviceId")
	}
	snapshot := make(graph, len(data.Blocks))
	for _, item := range data.Blocks {
		_, duplicate := snapshot[item.Id]
		if duplicate || item.Id == "" || !json.Valid([]byte(item.Metadata)) {
			return nil, "", echo.NewHTTPError(http.StatusBadGateway, "Validator returned an invalid workflow")
		}
		snapshot[item.Id] = block{Type: item.Type, Metadata: json.RawMessage(item.Metadata)}
	}
	return snapshot, serviceID, nil
}

func (h *handler) GetWorkflowRun(c echo.Context) error {
	runID, err := workflowhttp.UUID(c.Param("runId"), "runId")
	if err != nil {
		return err
	}
	current, err := loadRun(c.Request().Context(), h.db, runID, false)
	if err != nil {
		return runError(err)
	}
	return c.JSON(http.StatusOK, current.view())
}

func (h *handler) AcceptWorkflowRunEvent(c echo.Context) error {
	runID, err := workflowhttp.UUID(c.Param("runId"), "runId")
	if err != nil {
		return err
	}
	var body models.WorkflowRunEventRequest
	if err := workflowhttp.ReadJSON(c, &body); err != nil {
		return err
	}
	body.EventId, err = workflowhttp.UUID(body.EventId, "eventId")
	if err != nil {
		return err
	}
	isMessage := body.Text != "" && body.PaymentId == "" && body.PaymentStatus == ""
	isPayment := body.Text == "" && body.PaymentId != "" && (body.PaymentStatus == "succeeded" || body.PaymentStatus == "canceled")
	if !isMessage && !isPayment {
		return echo.NewHTTPError(http.StatusBadRequest, "Provide text or a paymentId with a final paymentStatus")
	}

	ctx := c.Request().Context()
	var current *run
	err = pgx.BeginFunc(ctx, h.db, func(tx pgx.Tx) error {
		var err error
		current, err = loadRun(ctx, tx, runID, true)
		if err != nil {
			return err
		}
		if previous, exists := current.Data.Events[body.EventId]; exists {
			if previous != body {
				return echo.NewHTTPError(http.StatusConflict, "eventId is already used with different data")
			}
			return nil
		}
		if err := current.accept(body); err != nil {
			return echo.NewHTTPError(http.StatusConflict, err.Error())
		}
		tag, err := tx.Exec(ctx, `
			UPDATE workflow.runs r
			SET current_block_id = NULLIF($3, ''), status = $4, variables = $5, state = $6, updated = now()
			FROM owners.services s
			WHERE r.id = $1 AND s.id = r.service_id AND s.owner_id = $2 AND NOT s.removed`,
			runID, workflowhttp.TemporaryUserID(ctx), current.BlockID, current.Status, current.Variables, current.Data)
		if err == nil && tag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return err
	})
	if err != nil {
		return runError(err)
	}
	return c.JSON(http.StatusOK, current.view())
}

type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func loadRun(ctx context.Context, db rowQuerier, id string, lock bool) (*run, error) {
	query := `SELECT r.id::text, r.workflow_id::text, r.entry_block_id, r.snapshot, COALESCE(r.current_block_id, ''), r.status, r.variables, r.state
		FROM workflow.runs r JOIN owners.services s ON s.id = r.service_id
		WHERE r.id = $1 AND s.owner_id = $2 AND NOT s.removed`
	if lock {
		query += " FOR UPDATE OF r"
	}
	var current run
	err := db.QueryRow(ctx, query, id, workflowhttp.TemporaryUserID(ctx)).Scan(
		&current.ID, &current.WorkflowID, &current.EntryBlock, &current.Graph,
		&current.BlockID, &current.Status, &current.Variables, &current.Data)
	if err != nil {
		return nil, fmt.Errorf("load run: %w", err)
	}
	return &current, nil
}

func runError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return echo.NewHTTPError(http.StatusNotFound, "Run not found")
	}
	return err
}
