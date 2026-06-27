package readmodel

import "encoding/json"

func ParseJSONObject(raw string) map[string]any {
	var value map[string]any
	if err := json.Unmarshal([]byte(raw), &value); err != nil || value == nil {
		return map[string]any{}
	}
	return value
}
