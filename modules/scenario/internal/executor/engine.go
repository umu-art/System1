package executor

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"unicode/utf8"

	"s1_workflow_api/models"
)

const (
	waitingMessage = "waiting_message"
	waitingPayment = "waiting_payment"
	completed      = "completed"
	failed         = "failed"
)

type block struct {
	Type     string          `json:"type"`
	Metadata json.RawMessage `json:"metadata"`
}

type graph map[string]block

type run struct {
	ID         string
	WorkflowID string
	EntryBlock string
	BlockID    string
	Status     string
	Variables  runVariables
	Graph      graph
	Data       runState
}

type runVariables struct {
	Input map[string]any `json:"input"`
	State map[string]any `json:"state"`
}

type runState struct {
	Error     string                                    `json:"error,omitempty"`
	PaymentID string                                    `json:"paymentId,omitempty"`
	Steps     int                                       `json:"steps"`
	Results   map[string]any                            `json:"results"`
	Messages  []map[string]any                          `json:"messages"`
	Events    map[string]models.WorkflowRunEventRequest `json:"events"`
}

type parameters struct {
	Scope       string
	Command     string
	SaveAs      string
	Next        string
	OnError     string
	OnSucceeded string
	OnCanceled  string
	Default     string
	Ref         string
	Text        string
	Media       any
	Value       any
	PaymentID   any
	Amount      map[string]any
	Input       []generationInput
	Cases       []condition
	Buttons     []map[string]any
}

type generationInput struct {
	Type  string
	Value any
}

type condition struct {
	Predicate string
	Ref       string
	Transform string
	Value     any
	Next      string
}

func newRun(id, workflowID, entry string, snapshot graph) *run {
	return &run{
		ID: id, WorkflowID: workflowID, EntryBlock: entry, Graph: snapshot,
		BlockID: entry, Variables: runVariables{Input: map[string]any{}, State: map[string]any{}},
		Data: runState{
			Results: map[string]any{}, Messages: []map[string]any{},
			Events: map[string]models.WorkflowRunEventRequest{},
		},
	}
}

func (r *run) view() models.WorkflowRunDto {
	return models.WorkflowRunDto{
		RunId: r.ID, WorkflowId: r.WorkflowID, Status: r.Status, BlockId: r.BlockID,
		Error: r.Data.Error, Input: r.Variables.Input, State: r.Variables.State, Results: r.Data.Results,
		Messages: r.Data.Messages,
	}
}

func (r *run) advance(eventText string) {
	for steps := 0; r.BlockID != ""; steps++ {
		if steps >= 128 || r.Data.Steps >= 10000 {
			r.fail(errors.New("Execution step limit exceeded"))
			return
		}
		id := r.BlockID
		current, exists := r.Graph[id]
		if !exists {
			r.fail(fmt.Errorf("Block %q is missing", id))
			return
		}
		var params parameters
		if err := json.Unmarshal(current.Metadata, &params); err != nil {
			r.fail(fmt.Errorf("Invalid metadata for block %q: %w", id, err))
			return
		}

		r.Data.Steps++
		r.Status = ""
		next, err := r.execute(id, current.Type, params, eventText)
		if err != nil {
			if params.OnError == "" {
				r.fail(err)
				return
			}
			next = params.OnError
		}
		if r.Status != "" {
			return
		}
		r.BlockID = next
	}
	r.Status = completed
}

func (r *run) execute(id, kind string, p parameters, eventText string) (string, error) {
	next := p.Next
	switch kind {
	case "command":
		if p.Scope == "run" {
			r.Status = waitingMessage
		}
	case "wait_message":
		r.Status = waitingMessage
	case "wait_payment":
		paymentID, err := r.paymentID(p, eventText)
		if err != nil {
			return "", err
		}
		r.Data.PaymentID = paymentID
		r.Status = waitingPayment
	case "send_message":
		message, err := r.message(p, eventText)
		if err != nil {
			return "", err
		}
		r.Data.Messages = append(r.Data.Messages, message)
	case "condition":
		return r.choose(p, eventText)
	case "set_value", "add_value":
		value, err := r.resolve(p.Value, eventText)
		if err != nil {
			return "", err
		}
		if err := r.writeVariable(kind, p.Ref, value); err != nil {
			return "", err
		}
	case "create_invoice":
		r.Data.Results[id] = r.mockInvoice(p.Amount)
	case "generate_audio", "generate_video":
		result, err := r.mockGeneration(kind, p.Input, eventText)
		if err != nil {
			return "", err
		}
		r.Data.Results[id] = result
	default:
		return "", fmt.Errorf("Unsupported block type %q", kind)
	}
	return next, nil
}

func (r *run) accept(event models.WorkflowRunEventRequest) error {
	id := r.BlockID
	current := r.Graph[id]
	if r.Status != waitingMessage && r.Status != waitingPayment {
		return errors.New("Run is not waiting for an event")
	}
	var p parameters
	if err := json.Unmarshal(current.Metadata, &p); err != nil {
		return fmt.Errorf("Invalid metadata for block %q", id)
	}
	switch r.Status {
	case waitingMessage:
		if event.Text == "" || (current.Type == "command" && event.Text != p.Command) {
			return errors.New("Run is waiting for a different message")
		}
		if current.Type == "wait_message" {
			r.Data.Results[id] = map[string]any{"text": event.Text}
			if p.SaveAs != "" {
				r.Variables.Input[p.SaveAs] = event.Text
			}
		}
		r.BlockID = p.Next
	case waitingPayment:
		if event.PaymentId != r.Data.PaymentID {
			return errors.New("Run is waiting for a different payment")
		}
		r.recordPayment(id, event)
		r.Data.PaymentID = ""
		r.BlockID = p.OnSucceeded
		if event.PaymentStatus == "canceled" {
			r.BlockID = p.OnCanceled
		}
	}
	r.Data.Events[event.EventId] = event
	r.advance(event.Text)
	return nil
}

func (r *run) fail(err error) {
	r.Status = failed
	r.Data.Error = err.Error()
}

func (r *run) choose(p parameters, eventText string) (string, error) {
	for _, branch := range p.Cases {
		value, err := r.read(branch.Ref, eventText)
		if err != nil {
			return "", err
		}
		if branch.Transform == "length" {
			text, ok := value.(string)
			if !ok {
				return "", errors.New("Length requires a string")
			}
			value = float64(utf8.RuneCountInString(text))
		}
		switch branch.Predicate {
		case "eq":
			if reflect.DeepEqual(value, branch.Value) {
				return branch.Next, nil
			}
		case "lt", "gt":
			left, leftOK := value.(float64)
			right, rightOK := branch.Value.(float64)
			if !leftOK || !rightOK {
				return "", errors.New("lt and gt require numbers")
			}
			if (branch.Predicate == "lt" && left < right) ||
				(branch.Predicate == "gt" && left > right) {
				return branch.Next, nil
			}
		}
	}
	return p.Default, nil
}

func (r *run) resolve(value any, eventText string) (any, error) {
	if ref, ok := value.(map[string]any); ok && len(ref) == 1 {
		if path, ok := ref["ref"].(string); ok {
			return r.read(path, eventText)
		}
	}
	return value, nil
}

func (r *run) read(path, eventText string) (any, error) {
	if path == "event.text" && eventText != "" {
		return eventText, nil
	}
	if suffix, ok := strings.CutPrefix(path, "input."); ok {
		return lookup(r.Variables.Input, suffix, path)
	}
	if suffix, ok := strings.CutPrefix(path, "state."); ok {
		return lookup(r.Variables.State, suffix, path)
	}
	for end := len(path); end > 0; {
		index := strings.LastIndex(path[:end], ".result")
		if index < 0 {
			break
		}
		id := path[:index]
		suffix := path[index+len(".result"):]
		if value, exists := r.Data.Results[id]; exists && (suffix == "" || strings.HasPrefix(suffix, ".")) {
			return lookup(value, strings.TrimPrefix(suffix, "."), path)
		}
		end = index
	}
	return nil, fmt.Errorf("Reference %q is undefined", path)
}

func lookup(value any, suffix, path string) (any, error) {
	if suffix == "" {
		return value, nil
	}
	for key := range strings.SplitSeq(suffix, ".") {
		mapping, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("Reference %q is undefined", path)
		}
		value, ok = mapping[key]
		if !ok {
			return nil, fmt.Errorf("Reference %q is undefined", path)
		}
	}
	return value, nil
}
