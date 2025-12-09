package models

import (
	"testing"
	"time"
)

func TestMetricValidation(t *testing.T) {
	tests := []struct {
		name    string
		metric  Metric
		wantErr bool
	}{
		{
			name: "valid metric",
			metric: Metric{
				Name:      "test_metric",
				Timestamp: time.Now(),
				Value:     42.0,
				Dimensions: map[string]string{
					"host": "server1",
				},
			},
			wantErr: false,
		},
		{
			name: "empty name",
			metric: Metric{
				Name:      "",
				Timestamp: time.Now(),
				Value:     42.0,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.metric.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
