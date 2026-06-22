package evaluation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
)

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

func LoadRawSampleFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func ExtractSampleArray(raw []byte) ([]json.RawMessage, error) {
	raw = bytes.TrimPrefix(raw, utf8BOM)

	var wrapped struct {
		Samples []json.RawMessage `json:"samples"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil && wrapped.Samples != nil {
		return wrapped.Samples, nil
	}

	var plain []json.RawMessage
	if err := json.Unmarshal(raw, &plain); err == nil {
		return plain, nil
	}
	return nil, fmt.Errorf("sample payload must be a JSON array or an object with samples")
}
