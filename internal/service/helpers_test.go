package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNullString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		valid bool
	}{
		{"non-empty", "hello", true},
		{"empty", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ns := nullString(tc.input)
			assert.Equal(t, tc.valid, ns.Valid)
			if tc.valid {
				assert.Equal(t, tc.input, ns.String)
			}
		})
	}
}

func TestNullInt32(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input int
		valid bool
	}{
		{"positive", 42, true},
		{"zero", 0, false},
		{"negative", -1, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ni := nullInt32(tc.input)
			assert.Equal(t, tc.valid, ni.Valid)
			if tc.valid {
				assert.Equal(t, int32(tc.input), ni.Int32)
			}
		})
	}
}

func TestNullFloat64(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input float64
		valid bool
	}{
		{"positive", 3.14, true},
		{"zero", 0.0, false},
		{"negative", -1.5, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			nf := nullFloat64(tc.input)
			assert.Equal(t, tc.valid, nf.Valid)
			if tc.valid {
				assert.Equal(t, tc.input, nf.Float64)
			}
		})
	}
}
