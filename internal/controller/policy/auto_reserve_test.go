package policy

import (
	"testing"

	v1alpha1 "github.com/martishin/k8s-node-disks-manager/api/v1alpha1"
	"github.com/martishin/k8s-node-disks-manager/internal/shared"
)

func TestShouldAutoReserve(t *testing.T) {
	tests := []struct {
		name    string
		labels  map[string]string
		desired v1alpha1.NodeDiskDesiredSpec
		want    bool
	}{
		{
			name:   "no label",
			labels: map[string]string{},
			want:   false,
		},
		{
			name: "with label and available",
			labels: map[string]string{
				shared.LabelAutoReserve: "true",
			},
			desired: v1alpha1.NodeDiskDesiredSpec{State: v1alpha1.DesiredStateAvailable},
			want:    true,
		},
		{
			name: "already auto reserved",
			labels: map[string]string{
				shared.LabelAutoReserve: "true",
			},
			desired: v1alpha1.NodeDiskDesiredSpec{State: v1alpha1.DesiredStateReserved, Owner: "auto"},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldAutoReserve(tt.labels, tt.desired); got != tt.want {
				t.Fatalf("ShouldAutoReserve() = %v, want %v", got, tt.want)
			}
		})
	}
}
