package schema

import (
	"google.golang.org/protobuf/reflect/protoreflect"
)

// ProtoToJSONSchema converts a protobuf MessageDescriptor to a JSON Schema map.
// The output uses lowerCamelCase field names to match protojson marshalling.
func ProtoToJSONSchema(md protoreflect.MessageDescriptor) map[string]any {
	return messageSchema(md, make(map[protoreflect.FullName]bool))
}

func messageSchema(md protoreflect.MessageDescriptor, seen map[protoreflect.FullName]bool) map[string]any {
	// Handle well-known types.
	switch md.FullName() {
	case "google.protobuf.Timestamp":
		return map[string]any{"type": "string", "format": "date-time"}
	case "google.protobuf.Duration":
		return map[string]any{"type": "string", "description": "Duration in seconds with 's' suffix, e.g. '3.5s'"}
	case "google.protobuf.Struct":
		return map[string]any{"type": "object"}
	case "google.protobuf.Value":
		return map[string]any{}
	case "google.protobuf.StringValue", "google.protobuf.BytesValue":
		return map[string]any{"type": "string"}
	case "google.protobuf.BoolValue":
		return map[string]any{"type": "boolean"}
	case "google.protobuf.Int32Value", "google.protobuf.Int64Value",
		"google.protobuf.UInt32Value", "google.protobuf.UInt64Value":
		return map[string]any{"type": "integer"}
	case "google.protobuf.FloatValue", "google.protobuf.DoubleValue":
		return map[string]any{"type": "number"}
	}

	// Guard against infinite recursion on self-referencing messages.
	if seen[md.FullName()] {
		return map[string]any{"type": "object", "description": string(md.FullName())}
	}
	seen[md.FullName()] = true
	defer func() { seen[md.FullName()] = false }()

	properties := map[string]any{}
	fields := md.Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		jsonName := fd.JSONName()
		properties[jsonName] = fieldSchema(fd, seen)
	}

	return map[string]any{
		"type":       "object",
		"title":      string(md.Name()),
		"properties": properties,
	}
}

func fieldSchema(fd protoreflect.FieldDescriptor, seen map[protoreflect.FullName]bool) map[string]any {
	if fd.IsMap() {
		valDesc := fd.MapValue()
		return map[string]any{
			"type":                 "object",
			"additionalProperties": scalarOrMessageSchema(valDesc, seen),
		}
	}

	schema := scalarOrMessageSchema(fd, seen)

	if fd.IsList() {
		return map[string]any{
			"type":  "array",
			"items": schema,
		}
	}

	return schema
}

func scalarOrMessageSchema(fd protoreflect.FieldDescriptor, seen map[protoreflect.FullName]bool) map[string]any {
	switch fd.Kind() {
	case protoreflect.BoolKind:
		return map[string]any{"type": "boolean"}
	case protoreflect.StringKind:
		return map[string]any{"type": "string"}
	case protoreflect.BytesKind:
		return map[string]any{"type": "string", "format": "byte"}
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind,
		protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return map[string]any{"type": "integer"}
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind,
		protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return map[string]any{"type": "integer"}
	case protoreflect.FloatKind, protoreflect.DoubleKind:
		return map[string]any{"type": "number"}
	case protoreflect.EnumKind:
		ed := fd.Enum()
		vals := ed.Values()
		names := make([]string, vals.Len())
		for i := 0; i < vals.Len(); i++ {
			names[i] = string(vals.Get(i).Name())
		}
		return map[string]any{"type": "string", "enum": names}
	case protoreflect.MessageKind, protoreflect.GroupKind:
		return messageSchema(fd.Message(), seen)
	default:
		return map[string]any{"type": "string"}
	}
}
