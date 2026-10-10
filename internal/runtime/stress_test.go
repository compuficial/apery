package runtime

import (
	"testing"

	"github.com/compuficial/apery/internal/rng"
)

const (
	stressIterations = 20
	stressSeed       = 99999
)

func TestStress(t *testing.T) {
	for _, tc := range canonicalPlans {
		t.Run(tc.name, func(t *testing.T) {
			baseline := runPlanWithOpts(t, tc.plan, WithWorkers(1), WithChunkSize(10000))
			baselineDigest := computeDigest(baseline)

			r := rng.New(rng.SeedFromInt64(stressSeed))
			for i := range stressIterations {
				workers := int(r.IntRange(1, 32))
				chunkSize := r.IntRange(1, 5000)

				actual := runPlanWithOpts(t, tc.plan, WithWorkers(workers), WithChunkSize(chunkSize))
				actualDigest := computeDigest(actual)

				if actualDigest != baselineDigest {
					basePath := dumpOutput(tc.name+".baseline", baseline)
					actPath := dumpOutput(tc.name+".actual", actual)
					t.Fatalf("iteration %d (workers=%d, chunkSize=%d): digest mismatch\n  baseline: %s\n  actual:   %s\n  diff: %s vs %s",
						i, workers, chunkSize, baselineDigest, actualDigest, basePath, actPath)
				}
			}

			t.Logf("passed %d iterations", stressIterations)
		})
	}
}
