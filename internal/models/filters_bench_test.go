package models_test

import (
	"testing"

	"github.com/piwi3910/netweave/internal/models"
)

// BenchmarkSelectFields_NoSelection benchmarks field selection with no fields specified.
func BenchmarkSelectFields_NoSelection(b *testing.B) {
	filter := &models.Filter{Fields: nil}
	data := map[string]interface{}{
		"id":             "resource-1",
		"name":           "Production models.Resource",
		"description":    "A production resource with many fields",
		"resourcePoolId": "pool-1",
		"extensions": map[string]interface{}{
			"cpu":    "8 cores",
			"memory": "32GB",
			"disk":   "500GB",
		},
		"metadata": map[string]interface{}{
			"labels": map[string]string{
				"env":  "prod",
				"tier": "premium",
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = filter.SelectFields(data)
	}
}

// BenchmarkSelectFields_TopLevel benchmarks top-level field selection.
func BenchmarkSelectFields_TopLevel(b *testing.B) {
	filter := &models.Filter{Fields: []string{"id", "name"}}
	data := map[string]interface{}{
		"id":             "resource-1",
		"name":           "Production models.Resource",
		"description":    "A production resource with many fields",
		"resourcePoolId": "pool-1",
		"extensions": map[string]interface{}{
			"cpu":    "8 cores",
			"memory": "32GB",
			"disk":   "500GB",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = filter.SelectFields(data)
	}
}

// BenchmarkSelectFields_Nested benchmarks nested field selection.
func BenchmarkSelectFields_Nested(b *testing.B) {
	filter := &models.Filter{Fields: []string{"extensions.cpu", "metadata.labels"}}
	data := map[string]interface{}{
		"id":   "resource-1",
		"name": "Production models.Resource",
		"extensions": map[string]interface{}{
			"cpu":    "8 cores",
			"memory": "32GB",
			"disk":   "500GB",
		},
		"metadata": map[string]interface{}{
			"labels": map[string]string{
				"env":  "prod",
				"tier": "premium",
			},
			"annotations": map[string]string{
				"owner": "team-a",
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = filter.SelectFields(data)
	}
}

// BenchmarkSelectFields_DeeplyNested benchmarks deeply nested field selection (5 levels).
func BenchmarkSelectFields_DeeplyNested(b *testing.B) {
	filter := &models.Filter{Fields: []string{"level1.level2.level3.level4.level5"}}
	data := map[string]interface{}{
		"id": "root",
		"level1": map[string]interface{}{
			"data": "level1-data",
			"level2": map[string]interface{}{
				"data": "level2-data",
				"level3": map[string]interface{}{
					"data": "level3-data",
					"level4": map[string]interface{}{
						"data": "level4-data",
						"level5": map[string]interface{}{
							"target": "found",
							"other":  "data",
						},
					},
				},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = filter.SelectFields(data)
	}
}

// BenchmarkSelectFields_MultipleNested benchmarks multiple nested field selections.
func BenchmarkSelectFields_MultipleNested(b *testing.B) {
	filter := &models.Filter{Fields: []string{
		"id",
		"extensions.cpu",
		"extensions.memory",
		"metadata.labels",
		"metadata.annotations",
	}}
	data := map[string]interface{}{
		"id":   "resource-1",
		"name": "Production models.Resource",
		"extensions": map[string]interface{}{
			"cpu":    "8 cores",
			"memory": "32GB",
			"disk":   "500GB",
			"gpu":    "2x NVIDIA",
		},
		"metadata": map[string]interface{}{
			"labels": map[string]string{
				"env":  "prod",
				"tier": "premium",
			},
			"annotations": map[string]string{
				"owner": "team-a",
				"cost":  "high",
			},
			"tags": []string{"critical", "monitored"},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = filter.SelectFields(data)
	}
}

// BenchmarkSelectFields_WithArrays benchmarks field selection with arrays.
func BenchmarkSelectFields_WithArrays(b *testing.B) {
	filter := &models.Filter{Fields: []string{"id", "items"}}
	data := map[string]interface{}{
		"id": "collection-1",
		"items": []interface{}{
			map[string]interface{}{"id": "1", "name": "item1", "value": 100},
			map[string]interface{}{"id": "2", "name": "item2", "value": 200},
			map[string]interface{}{"id": "3", "name": "item3", "value": 300},
			map[string]interface{}{"id": "4", "name": "item4", "value": 400},
			map[string]interface{}{"id": "5", "name": "item5", "value": 500},
		},
		"metadata": map[string]interface{}{
			"count": 5,
			"total": 1500,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = filter.SelectFields(data)
	}
}

// BenchmarkDeepCopy_Simple benchmarks deep copy of simple data.
func BenchmarkDeepCopy_Simple(b *testing.B) {
	data := map[string]interface{}{
		"id":    "resource-1",
		"name":  "Production models.Resource",
		"value": 42,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = models.DeepCopyValue(data)
	}
}

// BenchmarkDeepCopy_Complex benchmarks deep copy of complex nested data.
func BenchmarkDeepCopy_Complex(b *testing.B) {
	data := map[string]interface{}{
		"id":   "resource-1",
		"name": "Production models.Resource",
		"extensions": map[string]interface{}{
			"cpu":    "8 cores",
			"memory": "32GB",
			"nested": map[string]interface{}{
				"deep": map[string]interface{}{
					"value": "very deep",
				},
			},
		},
		"items": []interface{}{
			map[string]interface{}{"id": "1", "name": "item1"},
			map[string]interface{}{"id": "2", "name": "item2"},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = models.DeepCopyValue(data)
	}
}

// BenchmarkDeepCopy_LargeSlice benchmarks deep copy with large slice.
func BenchmarkDeepCopy_LargeSlice(b *testing.B) {
	items := make([]interface{}, 100)
	for i := 0; i < 100; i++ {
		items[i] = map[string]interface{}{
			"id":    i,
			"name":  "item",
			"value": i * 100,
		}
	}

	data := map[string]interface{}{
		"items": items,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = models.DeepCopyValue(data)
	}
}
