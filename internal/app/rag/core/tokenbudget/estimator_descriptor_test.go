package tokenbudget

import "testing"

func TestDescribeEstimatorReportsDefaultNameAndVersion(t *testing.T) {
	name, version := DescribeEstimator(NewDefaultEstimator())
	if name != "tokenestimate" || version != "v0.1.0" {
		t.Fatalf("DescribeEstimator() = %q %q", name, version)
	}
}
