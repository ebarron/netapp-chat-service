package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type canvasPayloadIssue struct {
	Code string
	Path string
}

func (i *canvasPayloadIssue) Error() string {
	return fmt.Sprintf("%s at %s", i.Code, i.Path)
}

type canvasPayloadMetadata struct {
	PayloadType string
	Kind        string
	Title       string
	Qualifier   string
	TabID       string
}

func rejectCanvas(code, path string) error {
	return &canvasPayloadIssue{Code: code, Path: path}
}

func asObject(value any) (map[string]any, bool) {
	obj, ok := value.(map[string]any)
	return obj, ok
}

func requireString(obj map[string]any, key, path string) error {
	if _, ok := obj[key].(string); !ok {
		return rejectCanvas("missing_or_invalid_string", path+"."+key)
	}
	return nil
}

func requireArray(obj map[string]any, key, path string) ([]any, error) {
	values, ok := obj[key].([]any)
	if !ok {
		return nil, rejectCanvas("missing_or_invalid_collection", path+"."+key)
	}
	return values, nil
}

func optionalArray(obj map[string]any, key string) []any {
	values, ok := obj[key].([]any)
	if !ok {
		values = []any{}
		obj[key] = values
	}
	return values
}

func requireObjectItems(values []any, path string) ([]map[string]any, error) {
	items := make([]map[string]any, 0, len(values))
	for i, value := range values {
		item, ok := asObject(value)
		if !ok {
			return nil, rejectCanvas("invalid_collection_item", fmt.Sprintf("%s[%d]", path, i))
		}
		items = append(items, item)
	}
	return items, nil
}

func inferCanvasPanelType(obj map[string]any) string {
	if _, ok := obj["xKey"].(string); ok {
		if _, seriesOK := obj["series"].([]any); seriesOK {
			if _, dataOK := obj["data"].([]any); dataOK {
				if _, area := obj["yLabel"].(string); area {
					return "area"
				}
				return "bar"
			}
		}
	}
	if _, valueOK := obj["value"].(float64); valueOK {
		if _, maxOK := obj["max"].(float64); maxOK {
			return "gauge"
		}
	}
	if data, ok := obj["data"].([]any); ok {
		if len(data) > 0 {
			if _, numeric := data[0].(float64); numeric {
				return "sparkline"
			}
		}
	}
	if _, columnsOK := obj["columns"].([]any); columnsOK {
		if _, rowsOK := obj["rows"].([]any); rowsOK {
			return "resource-table"
		}
	}
	if _, commandOK := obj["command"].(string); commandOK {
		if _, titleOK := obj["title"].(string); titleOK {
			return "proposal"
		}
	}
	if _, fieldsOK := obj["fields"].([]any); fieldsOK {
		if _, submitOK := asObject(obj["submit"]); submitOK {
			return "action-form"
		}
	}
	if buttons, ok := obj["buttons"].([]any); ok && len(buttons) > 0 {
		return "action-button"
	}
	if _, titleOK := obj["title"].(string); titleOK {
		if _, bodyOK := obj["body"].(string); bodyOK {
			return "callout"
		}
		if _, valueOK := obj["value"].(string); valueOK {
			return "stat"
		}
	}
	if data, ok := asObject(obj["data"]); ok {
		if _, critical := data["critical"].(float64); critical {
			return "alert-summary"
		}
		if _, warning := data["warning"].(float64); warning {
			return "alert-summary"
		}
	}
	if items, ok := obj["items"].([]any); ok && len(items) > 0 {
		if first, ok := asObject(items[0]); ok {
			if _, severity := first["severity"].(string); severity {
				return "alert-list"
			}
			if _, name := first["name"].(string); name {
				if _, status := first["status"].(string); status {
					return "status-grid"
				}
			}
		}
	}
	return ""
}

func validateCanvasPanel(panel map[string]any, path string) error {
	panelType, _ := panel["type"].(string)
	if panelType == "" {
		panelType = inferCanvasPanelType(panel)
		if panelType == "" {
			// Unknown panels are skipped by the client for forward compatibility.
			return nil
		}
		panel["type"] = panelType
	}

	switch panelType {
	case "area", "bar":
		if err := requireString(panel, "title", path); err != nil {
			return err
		}
		if err := requireString(panel, "xKey", path); err != nil {
			return err
		}
		series, err := requireArray(panel, "series", path)
		if err != nil {
			return err
		}
		seriesItems, err := requireObjectItems(series, path+".series")
		if err != nil {
			return err
		}
		for i, item := range seriesItems {
			if err := requireString(item, "key", fmt.Sprintf("%s.series[%d]", path, i)); err != nil {
				return err
			}
			if err := requireString(item, "label", fmt.Sprintf("%s.series[%d]", path, i)); err != nil {
				return err
			}
		}
		data, err := requireArray(panel, "data", path)
		if err != nil {
			return err
		}
		if _, err := requireObjectItems(data, path+".data"); err != nil {
			return err
		}
		if panelType == "area" {
			annotations := optionalArray(panel, "annotations")
			if _, err := requireObjectItems(annotations, path+".annotations"); err != nil {
				return err
			}
		}
	case "gauge":
		if err := requireString(panel, "title", path); err != nil {
			return err
		}
		if _, ok := panel["value"].(float64); !ok {
			return rejectCanvas("missing_or_invalid_number", path+".value")
		}
		if _, ok := panel["max"].(float64); !ok {
			return rejectCanvas("missing_or_invalid_number", path+".max")
		}
	case "sparkline":
		data, err := requireArray(panel, "data", path)
		if err != nil {
			return err
		}
		for i, value := range data {
			if _, ok := value.(float64); !ok {
				return rejectCanvas("invalid_collection_item", fmt.Sprintf("%s.data[%d]", path, i))
			}
		}
	case "status-grid":
		if err := requireString(panel, "title", path); err != nil {
			return err
		}
		items, err := requireArray(panel, "items", path)
		if err != nil {
			return err
		}
		records, err := requireObjectItems(items, path+".items")
		if err != nil {
			return err
		}
		for i, item := range records {
			if err := requireString(item, "name", fmt.Sprintf("%s.items[%d]", path, i)); err != nil {
				return err
			}
			if err := requireString(item, "status", fmt.Sprintf("%s.items[%d]", path, i)); err != nil {
				return err
			}
		}
	case "stat":
		if err := requireString(panel, "title", path); err != nil {
			return err
		}
		if err := requireString(panel, "value", path); err != nil {
			return err
		}
	case "alert-summary":
		if _, ok := asObject(panel["data"]); !ok {
			return rejectCanvas("missing_or_invalid_object", path+".data")
		}
	case "resource-table":
		if err := requireString(panel, "title", path); err != nil {
			return err
		}
		if _, err := requireArray(panel, "columns", path); err != nil {
			return err
		}
		rows, err := requireArray(panel, "rows", path)
		if err != nil {
			return err
		}
		if _, err := requireObjectItems(rows, path+".rows"); err != nil {
			return err
		}
	case "alert-list":
		items, err := requireArray(panel, "items", path)
		if err != nil {
			return err
		}
		if _, err := requireObjectItems(items, path+".items"); err != nil {
			return err
		}
	case "callout":
		if err := requireString(panel, "title", path); err != nil {
			return err
		}
		if err := requireString(panel, "body", path); err != nil {
			return err
		}
	case "proposal":
		if err := requireString(panel, "title", path); err != nil {
			return err
		}
		if err := requireString(panel, "command", path); err != nil {
			return err
		}
	case "action-button":
		buttons, err := requireArray(panel, "buttons", path)
		if err != nil {
			return err
		}
		records, err := requireObjectItems(buttons, path+".buttons")
		if err != nil {
			return err
		}
		for i, button := range records {
			buttonPath := fmt.Sprintf("%s.buttons[%d]", path, i)
			if err := requireString(button, "label", buttonPath); err != nil {
				return err
			}
			action, _ := button["action"].(string)
			tool, hasTool := button["tool"].(string)
			message, hasMessage := button["message"].(string)
			switch {
			case action == "execute" && hasTool && tool != "":
			case action == "message" && hasMessage && message != "":
			case action == "" && ((hasTool && tool != "") || (hasMessage && message != "")):
			default:
				return rejectCanvas("invalid_action_target", buttonPath)
			}
		}
	case "action-form":
		fields, err := requireArray(panel, "fields", path)
		if err != nil {
			return err
		}
		records, err := requireObjectItems(fields, path+".fields")
		if err != nil {
			return err
		}
		for i, field := range records {
			fieldPath := fmt.Sprintf("%s.fields[%d]", path, i)
			if err := requireString(field, "key", fieldPath); err != nil {
				return err
			}
			if err := requireString(field, "label", fieldPath); err != nil {
				return err
			}
			if err := requireString(field, "type", fieldPath); err != nil {
				return err
			}
			fieldType, _ := field["type"].(string)
			if fieldType != "text" && fieldType != "select" && fieldType != "checkbox" {
				return rejectCanvas("invalid_form_field_type", fieldPath+".type")
			}
			optionalArray(field, "options")
		}
		submit, ok := asObject(panel["submit"])
		if !ok {
			return rejectCanvas("missing_or_invalid_submit", path+".submit")
		}
		if err := requireString(submit, "label", path+".submit"); err != nil {
			return err
		}
		if err := requireString(submit, "tool", path+".submit"); err != nil {
			return err
		}
	default:
		// Explicit unknown types are retained for forward compatibility and
		// skipped by clients that do not understand them.
	}
	return nil
}

func validateCanvasDashboard(root map[string]any) error {
	if err := requireString(root, "title", "$"); err != nil {
		return err
	}
	panels, err := requireArray(root, "panels", "$")
	if err != nil {
		return err
	}
	records, err := requireObjectItems(panels, "$.panels")
	if err != nil {
		return err
	}
	for i, panel := range records {
		if err := validateCanvasPanel(panel, fmt.Sprintf("$.panels[%d]", i)); err != nil {
			return err
		}
	}
	return nil
}

func validateCanvasObjectDetail(root map[string]any) error {
	if err := requireString(root, "name", "$"); err != nil {
		return err
	}
	sections, err := requireArray(root, "sections", "$")
	if err != nil {
		return err
	}
	records, err := requireObjectItems(sections, "$.sections")
	if err != nil {
		return err
	}
	for i, section := range records {
		path := fmt.Sprintf("$.sections[%d]", i)
		if err := requireString(section, "title", path); err != nil {
			return err
		}
		if err := requireString(section, "layout", path); err != nil {
			return err
		}
		data, ok := asObject(section["data"])
		if !ok {
			return rejectCanvas("missing_or_invalid_object", path+".data")
		}
		layout, _ := section["layout"].(string)
		switch layout {
		case "properties":
			if _, err := requireArray(data, "items", path+".data"); err != nil {
				return err
			}
		case "chart":
			if _, ok := data["title"].(string); !ok {
				// The outer section already supplies the visible heading.
				data["title"] = ""
			}
			if err := validateCanvasPanel(data, path+".data"); err != nil {
				return err
			}
		case "alert-list":
			data["type"] = "alert-list"
			if err := validateCanvasPanel(data, path+".data"); err != nil {
				return err
			}
		case "timeline":
			if _, err := requireArray(data, "events", path+".data"); err != nil {
				return err
			}
		case "actions":
			data["type"] = "action-button"
			if err := validateCanvasPanel(data, path+".data"); err != nil {
				return err
			}
		case "table":
			data["type"] = "resource-table"
			if _, ok := data["title"].(string); !ok {
				data["title"] = ""
			}
			if err := validateCanvasPanel(data, path+".data"); err != nil {
				return err
			}
		case "text":
			if _, ok := data["body"].(string); !ok {
				data["body"] = ""
			}
		}
	}
	return nil
}

func normalizeCanvasPayload(raw string) (json.RawMessage, canvasPayloadMetadata, error) {
	var root map[string]any
	if err := json.Unmarshal([]byte(raw), &root); err != nil {
		return nil, canvasPayloadMetadata{}, rejectCanvas("invalid_json", "$")
	}

	meta := canvasPayloadMetadata{}
	if title, ok := root["name"].(string); ok {
		meta.Title = title
	}
	if title, ok := root["title"].(string); ok && meta.Title == "" {
		meta.Title = title
	}
	if kind, ok := root["kind"].(string); ok {
		meta.Kind = kind
	}
	if qualifier, ok := root["qualifier"].(string); ok {
		meta.Qualifier = qualifier
	}

	rootType, _ := root["type"].(string)
	switch {
	case root["close"] == true:
		if meta.Title == "" {
			return nil, meta, rejectCanvas("missing_close_identity", "$")
		}
		if rootType == "dashboard" {
			meta.PayloadType = "dashboard"
			meta.Kind = "dashboard"
		} else {
			meta.PayloadType = "object-detail"
		}
	case rootType == "dashboard" || root["panels"] != nil:
		meta.PayloadType = "dashboard"
		meta.Kind = "dashboard"
		if err := validateCanvasDashboard(root); err != nil {
			meta.TabID = fmt.Sprintf("%s::%s::%s", meta.Kind, meta.Title, meta.Qualifier)
			return nil, meta, err
		}
	case rootType == "object-detail" || root["sections"] != nil:
		meta.PayloadType = "object-detail"
		if err := validateCanvasObjectDetail(root); err != nil {
			meta.TabID = fmt.Sprintf("%s::%s::%s", meta.Kind, meta.Title, meta.Qualifier)
			return nil, meta, err
		}
	default:
		return nil, meta, rejectCanvas("unknown_canvas_content", "$.type")
	}

	meta.TabID = fmt.Sprintf("%s::%s::%s", meta.Kind, meta.Title, meta.Qualifier)
	normalized, err := json.Marshal(root)
	if err != nil {
		return nil, meta, rejectCanvas("normalization_failed", "$")
	}
	return normalized, meta, nil
}

func sameCanvasPayload(left, right json.RawMessage) bool {
	return bytes.Equal(left, right)
}
