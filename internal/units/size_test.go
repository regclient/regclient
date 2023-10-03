package units

import "testing"

func TestHuman(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		size   float64
		result string
	}{
		{
			name:   "zero",
			size:   0,
			result: "0.000B",
		},
		{
			name:   "1.024kB",
			size:   1024,
			result: "1.024kB",
		},
		{
			name:   "1MB",
			size:   1000099,
			result: "1.000MB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HumanSize(tt.size)
			if result != tt.result {
				t.Errorf("expected %s, received %s", tt.result, result)
			}
		})
	}
}
