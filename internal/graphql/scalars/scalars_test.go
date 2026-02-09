package scalars

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarshalTime(t *testing.T) {
	tests := []struct {
		name     string
		input    Time
		wantNull bool
		wantStr  string
	}{
		{
			name:     "valid time",
			input:    Time(time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)),
			wantNull: false,
			wantStr:  `"2024-06-15T10:30:00Z"`,
		},
		{
			name:     "zero time returns null",
			input:    Time(time.Time{}),
			wantNull: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := MarshalTime(tt.input)
			if tt.wantNull {
				// graphql.Null is a WriterFunc that writes "null"
				var buf bytes.Buffer
				m.MarshalGQL(&buf)
				assert.Equal(t, "null", buf.String())
				return
			}

			var buf bytes.Buffer
			m.MarshalGQL(&buf)
			assert.Equal(t, tt.wantStr, buf.String())
		})
	}
}

func TestUnmarshalTime(t *testing.T) {
	tests := []struct {
		name    string
		input   interface{}
		wantErr bool
		wantVal time.Time
	}{
		{
			name:    "valid RFC3339 string",
			input:   "2024-06-15T10:30:00Z",
			wantErr: false,
			wantVal: time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC),
		},
		{
			name:    "valid RFC3339 with offset",
			input:   "2024-06-15T10:30:00+02:00",
			wantErr: false,
			wantVal: time.Date(2024, 6, 15, 10, 30, 0, 0, time.FixedZone("", 2*60*60)),
		},
		{
			name:    "invalid format",
			input:   "not-a-date",
			wantErr: true,
		},
		{
			name:    "non-string type",
			input:   12345,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := UnmarshalTime(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.True(t, time.Time(result).Equal(tt.wantVal), "expected %v, got %v", tt.wantVal, time.Time(result))
		})
	}
}

func TestMarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   JSON
		wantStr string
	}{
		{
			name:    "simple object",
			input:   JSON{"key": "value"},
			wantStr: "{\"key\":\"value\"}\n",
		},
		{
			name:    "nested object",
			input:   JSON{"outer": map[string]interface{}{"inner": "value"}},
			wantStr: "{\"outer\":{\"inner\":\"value\"}}\n",
		},
		{
			name:    "empty object",
			input:   JSON{},
			wantStr: "{}\n",
		},
		{
			name:    "nil map",
			input:   nil,
			wantStr: "null\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := MarshalJSON(tt.input)
			var buf bytes.Buffer
			m.MarshalGQL(&buf)
			assert.Equal(t, tt.wantStr, buf.String())
		})
	}
}

func TestUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   interface{}
		wantErr bool
		wantVal JSON
	}{
		{
			name:    "valid map",
			input:   map[string]interface{}{"key": "value"},
			wantErr: false,
			wantVal: JSON{"key": "value"},
		},
		{
			name:    "non-map type string",
			input:   "not a map",
			wantErr: true,
		},
		{
			name:    "non-map type int",
			input:   42,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := UnmarshalJSON(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantVal, result)
		})
	}
}
