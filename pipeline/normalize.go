package pipeline

import (
	"encoding/json"
	"regexp"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// dateOnlyRe matches date-only strings like "2025-01-15".
var dateOnlyRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// normalizeTimestamps rewrites date-only strings (e.g. "2025-01-15") to full
// RFC 3339 timestamps ("2025-01-15T00:00:00Z") for any field whose proto type
// is google.protobuf.Timestamp. This lets smaller LLMs that return bare dates
// pass protojson unmarshalling.
func normalizeTimestamps(data []byte, md protoreflect.MessageDescriptor) ([]byte, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return data, nil // not a JSON object — return as-is
	}

	changed := false
	fields := md.Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		jsonName := fd.JSONName()

		raw, ok := obj[jsonName]
		if !ok {
			// Also try the proto field name (snake_case).
			jsonName = string(fd.Name())
			raw, ok = obj[jsonName]
			if !ok {
				continue
			}
		}

		if fd.Kind() == protoreflect.MessageKind && fd.Message().FullName() == "google.protobuf.Timestamp" {
			// Field is a Timestamp — check if the value is a bare date string.
			var s string
			if err := json.Unmarshal(raw, &s); err == nil {
				s = strings.TrimSpace(s)
				if dateOnlyRe.MatchString(s) {
					fixed, _ := json.Marshal(s + "T00:00:00Z")
					obj[jsonName] = fixed
					changed = true
				}
			}
		} else if fd.Kind() == protoreflect.MessageKind && !fd.IsList() && !fd.IsMap() {
			// Recurse into nested messages.
			nested, err := normalizeTimestamps(raw, fd.Message())
			if err == nil && string(nested) != string(raw) {
				obj[jsonName] = nested
				changed = true
			}
		}
	}

	if !changed {
		return data, nil
	}
	return json.Marshal(obj)
}
