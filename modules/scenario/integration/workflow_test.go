package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	runapi "s1_workflow_api/models"
	validatorapi "s1_workflow_validator_api/models"

	"system1/scenario/internal/executor"
	"system1/scenario/internal/validator"
)

func TestWorkflowServices(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL must point to a migrated PostgreSQL test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)

	ownerID, serviceID, otherOwnerID, otherServiceID := seedOwner(t, ctx, db)
	catalog, err := validator.LoadCatalog(repoPath("api", "src", "workflow-blocks.schema.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	validatorServer := httptest.NewServer(validator.NewServer(db, catalog))
	defer validatorServer.Close()
	call := func(method, path string, body, result any, status int) {
		t.Helper()
		request(t, validatorServer.URL, method, path, ownerID, body, result, status)
	}
	request(t, validatorServer.URL, "GET", "/v1/workflow/list", "", nil, nil, 200)
	request(t, validatorServer.URL, "GET", "/v1/workflow/list", "not-a-uuid", nil, nil, 400)
	request(t, validatorServer.URL, "GET", "/v1/workflow/list?userId="+ownerID+"&userId="+otherOwnerID,
		"", nil, nil, 400)
	call("GET", "/readyz", nil, nil, 200)

	var catalogDTO validatorapi.WorkflowBlockCatalogDto
	call("GET", "/v1/workflow/block/metadata", nil, &catalogDTO, 200)
	if len(catalogDTO.BlockTypes) != 10 {
		t.Fatalf("unexpected catalog: %d types", len(catalogDTO.BlockTypes))
	}
	call("POST", "/v1/workflow", map[string]string{"serviceId": otherServiceID}, nil, 404)

	var voice validatorapi.WorkflowDto
	var payment validatorapi.WorkflowDto
	for _, workflow := range []*validatorapi.WorkflowDto{&voice, &payment} {
		call("POST", "/v1/workflow", map[string]string{"serviceId": serviceID}, workflow, 201)
		if workflow.ServiceId != serviceID || workflow.WorkflowId == "" {
			t.Fatalf("unexpected workflow: %+v", workflow)
		}
	}
	var list validatorapi.WorkflowListDto
	call("GET", "/v1/workflow/list", nil, &list, 200)
	if len(list.Workflows) != 2 {
		t.Fatalf("expected two workflows, got %d", len(list.Workflows))
	}
	var otherList validatorapi.WorkflowListDto
	request(t, validatorServer.URL, "GET", "/v1/workflow/list", otherOwnerID, nil, &otherList, 200)
	if len(otherList.Workflows) != 0 {
		t.Fatalf("another user can see the workflows: %+v", otherList)
	}
	voicePath := "/v1/workflow/" + voice.WorkflowId
	request(t, validatorServer.URL, "GET", voicePath, "", nil, nil, 404)
	request(t, validatorServer.URL, "GET", voicePath, otherOwnerID, nil, nil, 404)
	call("GET", voicePath, nil, &voice, 200)
	call("PUT", voicePath, map[string]string{"serviceId": serviceID}, &voice, 200)
	call("PUT", voicePath, map[string]string{"serviceId": otherServiceID}, nil, 404)

	batch, err := os.ReadFile(repoPath("docs", "workflow", "workflow.example.json"))
	if err != nil {
		t.Fatal(err)
	}
	call("POST", voicePath+"/bulk/block", json.RawMessage(batch), nil, 204)

	var validation validatorapi.ValidationErrorDto
	call("POST", voicePath+"/bulk/block", graphBatch(
		block("temp", "send_message", `{"text":"valid"}`),
		block("broken", "command", `{"command":"/bad","scope":"entry","next":"missing"}`),
	), &validation, 422)
	if validation.Error != "VALIDATION_ERROR" || !strings.Contains(validation.Message, "missing") {
		t.Fatalf("unexpected error: %+v", validation)
	}
	call("GET", voicePath+"/block?blockId=temp", nil, nil, 404)
	call("DELETE", voicePath+"/block?blockId=voice.wait_text", nil, &validation, 422)
	call("GET", voicePath+"/block?blockId=voice.wait_text", nil, nil, 200)
	call("POST", voicePath+"/bulk/block", graphBatch(block("temp", "send_message", `{"text":"valid"}`)), nil, 204)
	call("DELETE", voicePath+"/block?blockId=temp", nil, nil, 200)

	var blocks validatorapi.WorkflowBlockListDto
	call("GET", voicePath+"/block/list", nil, &blocks, 200)
	if blocks.ServiceId != serviceID || len(blocks.Blocks) != 8 {
		t.Fatalf("unexpected block snapshot: service %q, %d blocks", blocks.ServiceId, len(blocks.Blocks))
	}

	runID, paymentRunID, canceledRunID, transferRunID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	defer func() {
		if _, err := db.Exec(context.Background(), "DELETE FROM workflow.runs WHERE id IN ($1, $2, $3, $4)", runID, paymentRunID, canceledRunID, transferRunID); err != nil {
			t.Error(err)
		}
	}()
	startBody := runapi.StartWorkflowRunRequest{RunId: runID, WorkflowId: voice.WorkflowId, EntryBlockId: "voice.start"}
	engine := startExecutor(t, dsn, validatorServer.URL)
	var state runapi.WorkflowRunDto
	request(t, engine.URL, "POST", "/api/workflow/runs", "", startBody, nil, 404)
	request(t, engine.URL, "POST", "/api/workflow/runs", otherOwnerID, startBody, nil, 404)
	request(t, engine.URL, "POST", "/api/workflow/runs", ownerID, startBody, &state, 200)
	if state.Status != "waiting_message" || state.BlockId != "voice.wait_text" || len(state.Messages) != 1 {
		t.Fatalf("run did not wait for text: %+v", state)
	}
	var updated validatorapi.WorkflowBlockDto
	call("PUT", voicePath+"/block?blockId=voice.deliver", validatorapi.UpdateWorkflowBlockRequest{
		Type: "send_message", Metadata: `{"text":"edited after the run started"}`,
	}, &updated, 200)
	if updated.Id != "voice.deliver" {
		t.Fatalf("wrong updated block: %+v", updated)
	}

	// The new HTTP server and connection pool recover the run from PostgreSQL.
	engine.Close()
	engine = startExecutor(t, dsn, validatorServer.URL)
	runPath := "/api/workflow/runs/" + runID
	request(t, engine.URL, "GET", runPath, "", nil, nil, 404)
	request(t, engine.URL, "GET", runPath, otherOwnerID, nil, nil, 404)
	request(t, engine.URL, "POST", runPath+"/events", otherOwnerID,
		runapi.WorkflowRunEventRequest{EventId: uuid.NewString(), Text: "Чужой ответ"}, nil, 404)
	state = runapi.WorkflowRunDto{}
	request(t, engine.URL, "GET", runPath, ownerID, nil, &state, 200)
	if state.Status != "waiting_message" || state.BlockId != "voice.wait_text" {
		t.Fatalf("waiting state lost after restart: %+v", state)
	}
	event := runapi.WorkflowRunEventRequest{EventId: uuid.NewString(), Text: "Да"}
	request(t, engine.URL, "POST", runPath+"/events", ownerID, event, &state, 200)
	if state.Status != "waiting_message" || len(state.Messages) != 2 {
		t.Fatalf("short text did not take retry branch: %+v", state)
	}
	sameEvent := event
	sameEvent.Text = "Ответ с изменённым содержимым"
	request(t, engine.URL, "POST", runPath+"/events", ownerID, sameEvent, nil, 409)

	event = runapi.WorkflowRunEventRequest{EventId: uuid.NewString(), Text: "Привет, Александр!"}
	var parallel sync.WaitGroup
	for range 2 {
		parallel.Go(func() {
			var completed runapi.WorkflowRunDto
			request(t, engine.URL, "POST", runPath+"/events", ownerID, event, &completed, 200)
			media, _ := completed.Results["voice.generate"].(map[string]any)
			if completed.Status != "completed" || len(completed.Messages) != 3 ||
				completed.Messages[2]["media"] == nil || media["kind"] != "audio" || media["assetId"] == "" {
				t.Errorf("resumed run did not produce one audio result using the saved graph: %+v", completed)
			}
		})
	}
	parallel.Wait()
	state = runapi.WorkflowRunDto{}
	request(t, engine.URL, "POST", "/api/workflow/runs", ownerID, startBody, &state, 200)
	if state.Status != "completed" || state.BlockId != "" || state.Input["text"] != event.Text || len(state.Messages) != 3 {
		t.Fatalf("start retry changed run: %+v", state)
	}

	state = runapi.WorkflowRunDto{}
	paymentPath := "/v1/workflow/" + payment.WorkflowId
	call("POST", paymentPath+"/bulk/block", graphBatch(
		block("start", "command", `{"command":"/order","scope":"entry","next":"invoice"}`),
		block("invoice", "create_invoice", `{"shopId":"test","secretKey":"test","amount":{"value":"2.00","currency":"RUB"},"returnUrl":"https://example.org","next":"pay"}`),
		block("pay", "wait_payment", `{"paymentId":{"ref":"invoice.result.paymentId"},"onSucceeded":"set","onCanceled":"cancel"}`),
		block("set", "set_value", `{"ref":"state.balance","value":1,"next":"credit"}`),
		block("credit", "add_value", `{"ref":"state.balance","value":2,"next":"video"}`),
		block("video", "generate_video", `{"model":"test","input":[{"type":"img","value":"https://example.org/first.png"},{"type":"img","value":"https://example.org/last.png"}],"next":"done"}`),
		block("done", "send_message", `{"media":{"ref":"video.result"}}`),
		block("cancel", "send_message", `{"text":"Отмена"}`),
	), nil, 204)

	request(t, engine.URL, "POST", "/api/workflow/runs", ownerID, runapi.StartWorkflowRunRequest{
		RunId: paymentRunID, WorkflowId: payment.WorkflowId, EntryBlockId: "start",
	}, &state, 200)
	invoice, _ := state.Results["invoice"].(map[string]any)
	paymentID, _ := invoice["paymentId"].(string)
	if state.Status != "waiting_payment" || paymentID == "" {
		t.Fatalf("did not wait for payment: %+v", state)
	}

	paymentEvents := "/api/workflow/runs/" + paymentRunID + "/events"
	request(t, engine.URL, "POST", paymentEvents, ownerID, runapi.WorkflowRunEventRequest{
		EventId: uuid.NewString(), PaymentId: "incorrect", PaymentStatus: "succeeded",
	}, nil, 409)
	request(t, engine.URL, "POST", paymentEvents, ownerID, runapi.WorkflowRunEventRequest{
		EventId: uuid.NewString(), PaymentId: paymentID, PaymentStatus: "succeeded",
	}, &state, 200)
	state = runapi.WorkflowRunDto{}
	request(t, engine.URL, "GET", "/api/workflow/runs/"+paymentRunID, ownerID, nil, &state, 200)
	media, _ := state.Results["video"].(map[string]any)
	if state.Status != "completed" || state.State["balance"] != float64(3) || media["kind"] != "video" {
		t.Fatalf("unexpected paid run: %+v", state)
	}

	var canceled runapi.WorkflowRunDto
	request(t, engine.URL, "POST", "/api/workflow/runs", ownerID, runapi.StartWorkflowRunRequest{
		RunId: canceledRunID, WorkflowId: payment.WorkflowId, EntryBlockId: "start",
	}, &canceled, 200)
	canceledInvoice, _ := canceled.Results["invoice"].(map[string]any)
	canceledPaymentID, _ := canceledInvoice["paymentId"].(string)
	request(t, engine.URL, "POST", "/api/workflow/runs/"+canceledRunID+"/events", ownerID, runapi.WorkflowRunEventRequest{
		EventId: uuid.NewString(), PaymentId: canceledPaymentID, PaymentStatus: "canceled",
	}, &canceled, 200)
	if canceled.Status != "completed" || len(canceled.State) != 0 || len(canceled.Messages) != 1 ||
		canceled.Messages[0]["text"] != "Отмена" {
		t.Fatalf("unexpected canceled run: %+v", canceled)
	}

	transferStart := runapi.StartWorkflowRunRequest{RunId: transferRunID, WorkflowId: voice.WorkflowId, EntryBlockId: "voice.start"}
	request(t, engine.URL, "POST", "/api/workflow/runs", ownerID, transferStart, nil, 200)
	call("DELETE", "/v1/workflow?workflowId="+voice.WorkflowId, nil, nil, 200)
	call("GET", voicePath, nil, nil, 404)
	request(t, engine.URL, "GET", runPath, ownerID, nil, &state, 200)
	if state.Status != "completed" {
		t.Fatalf("deleting definition changed existing run: %+v", state)
	}

	transferPath := "/api/workflow/runs/" + transferRunID
	request(t, engine.URL, "GET", transferPath, otherOwnerID, nil, nil, 404)
	if _, err := db.Exec(ctx, "UPDATE owners.services SET owner_id = $1 WHERE id = $2", otherOwnerID, serviceID); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := db.Exec(context.Background(), "UPDATE owners.services SET owner_id = $1, removed = false WHERE id = $2", ownerID, serviceID); err != nil {
			t.Error(err)
		}
	}()
	answer := runapi.WorkflowRunEventRequest{EventId: uuid.NewString(), Text: "После передачи сервиса"}
	request(t, engine.URL, "GET", transferPath, ownerID, nil, nil, 404)
	request(t, engine.URL, "POST", transferPath+"/events", ownerID, answer, nil, 404)
	request(t, engine.URL, "POST", "/api/workflow/runs", otherOwnerID, transferStart, &state, 200)
	if state.Status != "waiting_message" {
		t.Fatalf("transfer changed the pending run: %+v", state)
	}
	request(t, engine.URL, "POST", transferPath+"/events", otherOwnerID, answer, &state, 200)
	if state.Status != "completed" {
		t.Fatalf("new service owner could not resume the saved graph: %+v", state)
	}
	if _, err := db.Exec(ctx, "UPDATE owners.services SET removed = true WHERE id = $1", serviceID); err != nil {
		t.Fatal(err)
	}
	request(t, engine.URL, "GET", transferPath, otherOwnerID, nil, nil, 404)
}

func repoPath(parts ...string) string {
	return filepath.Join(append([]string{"..", "..", ".."}, parts...)...)
}

func startExecutor(t *testing.T, dsn, validatorURL string) *httptest.Server {
	t.Helper()
	db, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	server, err := executor.NewServer(db, validatorURL, "test")
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server)
	t.Cleanup(func() {
		httpServer.Close()
		db.Close()
	})
	return httpServer
}

func seedOwner(t *testing.T, ctx context.Context, db *pgxpool.Pool) (string, string, string, string) {
	t.Helper()
	create := func() (string, string) {
		owner, service := uuid.NewString(), uuid.NewString()
		if _, err := db.Exec(ctx, `
			INSERT INTO owners.owners (id, username, first_name, last_name, email, password_hash)
			VALUES ($1, $2, 'Integration', 'Test', $3, 'unused')`, owner, owner, owner+"@example.invalid"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			for _, query := range []string{
				"DELETE FROM workflow_validator.workflows WHERE service_id = $1",
				"DELETE FROM owners.services WHERE id = $1",
			} {
				if _, err := db.Exec(context.Background(), query, service); err != nil {
					t.Error(err)
				}
			}
			if _, err := db.Exec(context.Background(), "DELETE FROM owners.owners WHERE id = $1", owner); err != nil {
				t.Error(err)
			}
		})
		if _, err := db.Exec(ctx, "INSERT INTO owners.services (id, owner_id, name) VALUES ($1, $2, 'Workflow test')", service, owner); err != nil {
			t.Fatal(err)
		}
		return owner, service
	}
	owner, service := create()
	otherOwner, otherService := create()
	return owner, service, otherOwner, otherService
}

func graphBatch(blocks ...validatorapi.WorkflowBlockDto) validatorapi.UpsertWorkflowBlocksRequest {
	return validatorapi.UpsertWorkflowBlocksRequest{Blocks: blocks}
}

func block(id, kind, metadata string) validatorapi.WorkflowBlockDto {
	return validatorapi.WorkflowBlockDto{Id: id, Type: kind, Metadata: metadata}
}

func request(t *testing.T, base, method, path, userID string, body, result any, status int) {
	t.Helper()
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			t.Error(err)
			return
		}
	}
	req, err := http.NewRequest(method, base+path, bytes.NewReader(payload))
	if err != nil {
		t.Error(err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if userID != "" {
		query := req.URL.Query()
		query.Set("userId", userID)
		req.URL.RawQuery = query.Encode()
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Error(err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != status {
		data, _ := io.ReadAll(resp.Body)
		t.Errorf("%s %s: got %d, want %d: %s", method, path, resp.StatusCode, status, data)
		return
	}
	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			t.Error(err)
		}
	}
}
