package tools

import (
	"context"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"testing"
)

func TestCalculate(t *testing.T) {
	tests := []struct {
		expr string
		want float64
	}{
		{"sqrt(16) + 2*3", 10},
		{"pow(2,10)", 1024},
		{"sin(pi/2) * sin(pi/2) + cos(pi/2) * cos(pi/2)", 1},
	}
	for _, tt := range tests {
		out, err := NewCalculateTool().Execute(context.Background(), mustArgs(t, map[string]any{"expression": tt.expr}))
		if err != nil {
			t.Fatalf("calculate %q failed: %v", tt.expr, err)
		}
		got, err := strconv.ParseFloat(strings.TrimSpace(out), 64)
		if err != nil {
			t.Fatalf("parse result %q: %v", out, err)
		}
		if math.Abs(got-tt.want) > 1e-9 {
			t.Fatalf("calculate %q = %g, want %g", tt.expr, got, tt.want)
		}
	}
}

func TestStatisticsValues(t *testing.T) {
	out, err := NewStatisticsTool().Execute(context.Background(), mustArgs(t, map[string]any{
		"values": []float64{1, 2, 3, 4},
	}))
	if err != nil {
		t.Fatalf("statistics failed: %v", err)
	}
	idx := strings.Index(out, "{")
	if idx < 0 {
		t.Fatalf("statistics output did not include JSON: %q", out)
	}
	var got map[string]float64
	if err := json.Unmarshal([]byte(out[idx:]), &got); err != nil {
		t.Fatalf("parse statistics JSON: %v\n%s", err, out)
	}
	want := map[string]float64{
		"count":    4,
		"sum":      10,
		"mean":     2.5,
		"median":   2.5,
		"min":      1,
		"max":      4,
		"variance": 1.6666666666666667,
		"stddev":   math.Sqrt(1.6666666666666667),
	}
	for op, expected := range want {
		if math.Abs(got[op]-expected) > 1e-9 {
			t.Fatalf("%s = %g, want %g (all results: %#v)", op, got[op], expected, got)
		}
	}
}
