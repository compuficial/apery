package runtime

import "testing"

func TestGolden(t *testing.T) {
	for _, tc := range canonicalPlans {
		t.Run(tc.name, func(t *testing.T) {
			data := runPlanWithOpts(t, tc.plan, WithWorkers(1), WithChunkSize(10000))

			if *update {
				writeGolden(t, tc.name, data)
				return
			}

			compareGolden(t, tc.name, data)
		})
	}
}
