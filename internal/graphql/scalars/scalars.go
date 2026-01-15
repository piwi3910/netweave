// Package scalars provides custom scalar types for GraphQL.
package scalars

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/99designs/gqlgen/graphql"
)

// Time is a custom scalar type for time.Time.
type Time time.Time

// MarshalTime marshals Time to GraphQL (RFC3339 format).
func MarshalTime(t Time) graphql.Marshaler {
	if time.Time(t).IsZero() {
		return graphql.Null
	}
	return graphql.WriterFunc(func(w io.Writer) {
		_, _ = io.WriteString(w, fmt.Sprintf(`"%s"`, time.Time(t).Format(time.RFC3339)))
	})
}

// UnmarshalTime unmarshals GraphQL Time scalar.
func UnmarshalTime(v interface{}) (Time, error) {
	if str, ok := v.(string); ok {
		t, err := time.Parse(time.RFC3339, str)
		return Time(t), err
	}
	return Time{}, fmt.Errorf("time must be a string in RFC3339 format")
}

// JSON is a custom scalar type for JSON objects.
type JSON map[string]interface{}

// MarshalJSON marshals JSON to GraphQL.
func MarshalJSON(v JSON) graphql.Marshaler {
	return graphql.WriterFunc(func(w io.Writer) {
		if err := json.NewEncoder(w).Encode(v); err != nil {
			_, _ = io.WriteString(w, "null")
		}
	})
}

// UnmarshalJSON unmarshals GraphQL JSON scalar.
func UnmarshalJSON(v interface{}) (JSON, error) {
	if m, ok := v.(map[string]interface{}); ok {
		return JSON(m), nil
	}
	return nil, fmt.Errorf("JSON must be an object")
}
