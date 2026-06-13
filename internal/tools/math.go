package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/xuri/excelize/v2"
)

type calculateTool struct{}
type statisticsTool struct{}

type calculateArgs struct {
	Expression string `json:"expression"`
}

type statisticsArgs struct {
	Values     []float64         `json:"values"`
	Source     *statisticsSource `json:"source"`
	Operations []string          `json:"operations"`
}

type statisticsSource struct {
	Type   string `json:"type"`
	Path   string `json:"path"`
	Sheet  string `json:"sheet"`
	Column any    `json:"column"`
}

// NewCalculateTool returns an expression calculator.
func NewCalculateTool() Tool { return calculateTool{} }

func (calculateTool) Name() string { return "calculate" }

func (calculateTool) Description() string {
	return "Evaluate a mathematical expression with common math functions and constants."
}

func (calculateTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"expression": map[string]any{"type": "string", "description": "Mathematical expression to evaluate."},
		},
		"required": []string{"expression"},
	}
}

func (calculateTool) Mutating() bool { return false }

func (calculateTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	var parsed calculateArgs
	if err := json.Unmarshal(args, &parsed); err != nil {
		return "", fmt.Errorf("invalid calculate arguments: %w", err)
	}
	if strings.TrimSpace(parsed.Expression) == "" {
		return "", fmt.Errorf("expression is required")
	}

	env := mathEnv()
	program, err := expr.Compile(parsed.Expression, expr.Env(env))
	if err != nil {
		return "", fmt.Errorf("could not compile expression %q: %w", parsed.Expression, err)
	}
	result, err := expr.Run(program, env)
	if err != nil {
		return "", fmt.Errorf("could not evaluate expression %q: %w", parsed.Expression, err)
	}
	return formatCalcResult(result), nil
}

func mathEnv() map[string]any {
	return map[string]any{
		"pi":    math.Pi,
		"e":     math.E,
		"sqrt":  math.Sqrt,
		"abs":   math.Abs,
		"pow":   math.Pow,
		"exp":   math.Exp,
		"log":   math.Log,
		"ln":    math.Log,
		"log10": math.Log10,
		"log2":  math.Log2,
		"sin":   math.Sin,
		"cos":   math.Cos,
		"tan":   math.Tan,
		"asin":  math.Asin,
		"acos":  math.Acos,
		"atan":  math.Atan,
		"atan2": math.Atan2,
		"floor": math.Floor,
		"ceil":  math.Ceil,
		"round": math.Round,
		"trunc": math.Trunc,
		"min":   math.Min,
		"max":   math.Max,
		"hypot": math.Hypot,
		"mod":   math.Mod,
	}
}

func formatCalcResult(v any) string {
	switch n := v.(type) {
	case float64:
		return fmt.Sprintf("%g", n)
	case float32:
		return fmt.Sprintf("%g", n)
	case int:
		return strconv.Itoa(n)
	case int64:
		return strconv.FormatInt(n, 10)
	case int32:
		return strconv.FormatInt(int64(n), 10)
	case uint:
		return strconv.FormatUint(uint64(n), 10)
	case uint64:
		return strconv.FormatUint(n, 10)
	default:
		return fmt.Sprint(v)
	}
}

// NewStatisticsTool returns a tool that computes basic statistics from values or a data source.
func NewStatisticsTool() Tool { return statisticsTool{} }

func (statisticsTool) Name() string { return "statistics" }

func (statisticsTool) Description() string {
	return "Compute count, sum, mean, median, min, max, sample variance, and sample standard deviation."
}

func (statisticsTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"values": map[string]any{"type": "array", "items": map[string]any{"type": "number"}, "description": "Numbers to summarize."},
			"source": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"type":   map[string]any{"type": "string", "enum": []string{"json", "excel"}},
					"path":   map[string]any{"type": "string"},
					"sheet":  map[string]any{"type": "string"},
					"column": map[string]any{"description": "Header/key name or numeric column index."},
				},
				"required": []string{"type", "path"},
			},
			"operations": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Operations to compute. Defaults to all."},
		},
	}
}

func (statisticsTool) Mutating() bool { return false }

func (statisticsTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	var parsed statisticsArgs
	if err := json.Unmarshal(args, &parsed); err != nil {
		return "", fmt.Errorf("invalid statistics arguments: %w", err)
	}

	values := parsed.Values
	if len(values) == 0 {
		if parsed.Source == nil {
			return "", fmt.Errorf("provide values or source")
		}
		loaded, err := loadStatisticsSource(*parsed.Source)
		if err != nil {
			return "", err
		}
		values = loaded
	}
	if len(values) == 0 {
		return "", fmt.Errorf("no numeric values found")
	}

	results, err := computeStatistics(values, parsed.Operations)
	if err != nil {
		return "", err
	}
	pretty, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Statistics for %d value(s):\n", len(values))
	for _, op := range orderedOps(parsed.Operations) {
		if v, ok := results[op]; ok {
			fmt.Fprintf(&b, "%s: %g\n", op, v)
		}
	}
	b.WriteString("\n")
	b.Write(pretty)
	return b.String(), nil
}

func loadStatisticsSource(source statisticsSource) ([]float64, error) {
	if source.Path == "" {
		return nil, fmt.Errorf("source.path is required")
	}
	switch strings.ToLower(source.Type) {
	case "json":
		return loadJSONNumbers(source.Path, source.Column)
	case "excel":
		return loadExcelNumbers(source.Path, source.Sheet, source.Column)
	default:
		return nil, fmt.Errorf("unsupported source.type %q; expected json or excel", source.Type)
	}
}

func loadJSONNumbers(path string, column any) ([]float64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read json source %q: %w", path, err)
	}
	var nums []float64
	if err := json.Unmarshal(data, &nums); err == nil {
		return nums, nil
	}
	var objects []map[string]any
	if err := json.Unmarshal(data, &objects); err != nil {
		return nil, fmt.Errorf("json source must be an array of numbers or objects: %w", err)
	}
	key, ok := column.(string)
	if !ok || key == "" {
		return nil, fmt.Errorf("source.column is required for json arrays of objects")
	}
	for _, obj := range objects {
		if v, ok := toFloat64(obj[key]); ok {
			nums = append(nums, v)
		}
	}
	return nums, nil
}

func loadExcelNumbers(path, sheet string, column any) ([]float64, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("open excel source %q: %w", path, err)
	}
	defer f.Close()
	if sheet == "" {
		sheets := f.GetSheetList()
		if len(sheets) == 0 {
			return nil, fmt.Errorf("excel source %q has no sheets", path)
		}
		sheet = sheets[0]
	}
	rows, err := f.GetRows(sheet)
	if err != nil {
		return nil, fmt.Errorf("read sheet %q: %w", sheet, err)
	}
	if len(rows) == 0 {
		return nil, nil
	}

	col, err := excelColumnIndex(rows[0], column)
	if err != nil {
		return nil, err
	}
	startRow := 0
	if _, ok := column.(string); ok {
		startRow = 1
	}
	var nums []float64
	for _, row := range rows[startRow:] {
		if col >= 0 && col < len(row) {
			if v, err := strconv.ParseFloat(strings.TrimSpace(row[col]), 64); err == nil {
				nums = append(nums, v)
			}
		}
	}
	return nums, nil
}

func excelColumnIndex(header []string, column any) (int, error) {
	if column == nil {
		return 0, nil
	}
	switch c := column.(type) {
	case string:
		if c == "" {
			return 0, nil
		}
		for i, h := range header {
			if strings.EqualFold(strings.TrimSpace(h), strings.TrimSpace(c)) {
				return i, nil
			}
		}
		return 0, fmt.Errorf("excel column header %q not found", c)
	case float64:
		return normalizeColumnIndex(int(c)), nil
	default:
		return 0, fmt.Errorf("unsupported source.column type %T", column)
	}
}

func normalizeColumnIndex(i int) int {
	if i <= 0 {
		return 0
	}
	return i - 1
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func computeStatistics(values []float64, operations []string) (map[string]float64, error) {
	results := make(map[string]float64)
	requested := orderedOps(operations)
	sortedValues := append([]float64(nil), values...)
	sort.Float64s(sortedValues)

	sum := 0.0
	for _, v := range values {
		sum += v
	}
	for _, op := range requested {
		switch strings.ToLower(op) {
		case "count":
			results["count"] = float64(len(values))
		case "sum":
			results["sum"] = sum
		case "mean":
			results["mean"] = sum / float64(len(values))
		case "median":
			results["median"] = median(sortedValues)
		case "min":
			results["min"] = sortedValues[0]
		case "max":
			results["max"] = sortedValues[len(sortedValues)-1]
		case "variance":
			results["variance"] = sampleVariance(values, sum/float64(len(values)))
		case "stddev":
			results["stddev"] = math.Sqrt(sampleVariance(values, sum/float64(len(values))))
		default:
			return nil, fmt.Errorf("unsupported operation %q", op)
		}
	}
	return results, nil
}

func orderedOps(operations []string) []string {
	all := []string{"count", "sum", "mean", "median", "min", "max", "variance", "stddev"}
	if len(operations) == 0 {
		return all
	}
	out := make([]string, 0, len(operations))
	for _, op := range operations {
		out = append(out, strings.ToLower(op))
	}
	return out
}

func median(sortedValues []float64) float64 {
	n := len(sortedValues)
	if n%2 == 1 {
		return sortedValues[n/2]
	}
	return (sortedValues[n/2-1] + sortedValues[n/2]) / 2
}

func sampleVariance(values []float64, mean float64) float64 {
	if len(values) < 2 {
		return 0
	}
	sumSquares := 0.0
	for _, v := range values {
		diff := v - mean
		sumSquares += diff * diff
	}
	return sumSquares / float64(len(values)-1)
}
