package driverapi

import (
	"fmt"

	"github.com/AnqorDX/vdb-core/internal/payloads"
)

func validateQueryResult(v any) (string, error) {
	switch t := v.(type) {
	case payloads.QueryReceivedPayload:
		return t.Query, nil
	case map[string]any:
		q, ok := t["Query"].(string)
		if !ok {
			return "", fmt.Errorf("driverapi: query.received result: 'Query' field missing or not a string (got %T)", t["Query"])
		}
		return q, nil
	default:
		return "", fmt.Errorf("driverapi: query.received result has unrecognised type %T", v)
	}
}

func validateRecordsSourceResult(v any) ([]map[string]any, error) {
	switch t := v.(type) {
	case payloads.RecordsSourcePayload:
		return t.Records, nil
	case map[string]any:
		return extractRecordSlice(t, "Records")
	default:
		return nil, fmt.Errorf("driverapi: records.source result has unrecognised type %T", v)
	}
}

func validateRecordsMergedResult(v any) ([]map[string]any, error) {
	switch t := v.(type) {
	case payloads.RecordsMergedPayload:
		return t.Records, nil
	case map[string]any:
		return extractRecordSlice(t, "Records")
	default:
		return nil, fmt.Errorf("driverapi: records.merged result has unrecognised type %T", v)
	}
}

func validateWriteInsertResult(v any) (map[string]any, error) {
	switch t := v.(type) {
	case payloads.WriteInsertPayload:
		return t.Record, nil
	case map[string]any:
		return extractRecord(t, "Record")
	default:
		return nil, fmt.Errorf("driverapi: write.insert result has unrecognised type %T", v)
	}
}

func validateWriteUpdateResult(v any) (map[string]any, error) {
	switch t := v.(type) {
	case payloads.WriteUpdatePayload:
		return t.NewRecord, nil
	case map[string]any:
		return extractRecord(t, "NewRecord")
	default:
		return nil, fmt.Errorf("driverapi: write.update result has unrecognised type %T", v)
	}
}

func extractRecord(m map[string]any, field string) (map[string]any, error) {
	v, ok := m[field]
	if !ok {
		return nil, fmt.Errorf("result map missing field %q", field)
	}
	rec, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("result map field %q is not a record (got %T)", field, v)
	}
	return rec, nil
}

func extractRecordSlice(m map[string]any, field string) ([]map[string]any, error) {
	v, ok := m[field]
	if !ok {
		return nil, fmt.Errorf("result map missing field %q", field)
	}
	switch t := v.(type) {
	case []map[string]any:
		return t, nil
	case []any:
		out := make([]map[string]any, len(t))
		for i, item := range t {
			rec, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("result map field %q element %d is not a record (got %T)", field, i, item)
			}
			out[i] = rec
		}
		return out, nil
	default:
		return nil, fmt.Errorf("result map field %q is not a record slice (got %T)", field, v)
	}
}
