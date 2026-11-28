package executor

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"s1_workflow_api/models"
)

func (r *run) writeVariable(kind, path string, value any) error {
	namespace, suffix, _ := strings.Cut(path, ".")
	target := r.Variables.Input
	switch namespace {
	case "input":
	case "state":
		target = r.Variables.State
	default:
		return fmt.Errorf("Invalid variable path %q", path)
	}
	root := maps.Clone(target)
	target = root
	keys := strings.Split(suffix, ".")
	for _, key := range keys[:len(keys)-1] {
		child, exists := target[key]
		if !exists {
			child = map[string]any{}
		}
		mapping, ok := child.(map[string]any)
		if !ok {
			return fmt.Errorf("Variable path %q crosses a scalar", path)
		}
		copy := maps.Clone(mapping)
		target[key] = copy
		target = copy
	}
	key := keys[len(keys)-1]
	if kind == "add_value" {
		delta, ok := value.(float64)
		if !ok {
			return errors.New("add_value requires a number")
		}
		if current, exists := target[key]; exists {
			number, ok := current.(float64)
			if !ok {
				return fmt.Errorf("Variable %q is not a number", path)
			}
			delta += number
		}
		if math.IsInf(delta, 0) {
			return errors.New("add_value overflowed")
		}
		value = delta
	}
	target[key] = value
	if namespace == "input" {
		r.Variables.Input = root
	} else {
		r.Variables.State = root
	}
	return nil
}

func (r *run) message(p parameters, eventText string) (map[string]any, error) {
	message := map[string]any{"blockId": r.BlockID}
	if p.Text != "" {
		message["text"] = p.Text
	} else {
		value, err := r.resolve(p.Media, eventText)
		if err != nil {
			return nil, err
		}
		media, ok := value.(map[string]any)
		if !ok || media["kind"] == nil {
			return nil, errors.New("send_message media reference must resolve to a media object")
		}
		message["media"] = media
	}

	if len(p.Buttons) != 0 {
		buttons := make([]map[string]any, 0, len(p.Buttons))
		for _, spec := range p.Buttons {
			button := maps.Clone(spec)
			if rawURL, exists := spec["url"]; exists {
				value, err := r.resolve(rawURL, eventText)
				if err != nil {
					return nil, err
				}
				link, ok := value.(string)
				if !ok || link == "" {
					return nil, errors.New("Button URL reference must resolve to a URL")
				}
				parsed, err := url.Parse(link)
				if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http" && parsed.Scheme != "tg") {
					return nil, errors.New("Button URL must use HTTP, HTTPS or tg")
				}
				button["url"] = link
			}
			buttons = append(buttons, button)
		}
		message["buttons"] = buttons
	}
	return message, nil
}

func (r *run) mockGeneration(kind string, inputs []generationInput, eventText string) (map[string]any, error) {
	for _, input := range inputs {
		value, err := r.resolve(input.Value, eventText)
		if err != nil {
			return nil, err
		}
		text, isText := value.(string)
		if input.Type == "text" {
			if !isText || text == "" {
				return nil, errors.New("Text input must resolve to a nonempty string")
			}
			continue
		}
		if _, fromReference := input.Value.(map[string]any); fromReference {
			media, ok := value.(map[string]any)
			mediaKind, _ := media["kind"].(string)
			if !ok || (mediaKind != input.Type && !(input.Type == "img" && mediaKind == "image")) {
				return nil, fmt.Errorf("Input must resolve to %s media", input.Type)
			}
		}
	}

	mime, mediaKind := "audio/wav", "audio"
	if kind == "generate_video" {
		mime, mediaKind = "video/mp4", "video"
	}
	return map[string]any{
		"assetId": r.operationID(), "kind": mediaKind, "mimeType": mime, "simulated": true,
	}, nil
}

func (r *run) mockInvoice(amount map[string]any) map[string]any {
	id := r.operationID()
	return map[string]any{
		"paymentId": id, "paymentUrl": "https://example.invalid/pay/" + id,
		"amount": amount, "simulated": true,
	}
}

func (r *run) operationID() string {
	return uuid.NewSHA1(uuid.MustParse(r.ID), []byte(fmt.Sprintf("%d", r.Data.Steps))).String()
}

func (r *run) paymentID(p parameters, eventText string) (string, error) {
	value, err := r.resolve(p.PaymentID, eventText)
	if err != nil {
		return "", err
	}
	id, ok := value.(string)
	if !ok || id == "" {
		return "", errors.New("wait_payment requires a payment identifier")
	}
	return id, nil
}

func (r *run) recordPayment(blockID string, event models.WorkflowRunEventRequest) {
	var amount any
	for _, value := range r.Data.Results {
		invoice, ok := value.(map[string]any)
		if ok && invoice["paymentId"] == event.PaymentId {
			amount = invoice["amount"]
			break
		}
	}
	r.Data.Results[blockID] = map[string]any{
		"paymentId": event.PaymentId, "status": event.PaymentStatus, "amount": amount,
	}
}
