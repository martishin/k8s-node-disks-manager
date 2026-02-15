package policy

import (
	v1alpha1 "github.com/martishin/k8s-node-disks-manager/api/v1alpha1"
	"github.com/martishin/k8s-node-disks-manager/internal/shared"
)

func ShouldAutoReserve(labels map[string]string, desired v1alpha1.NodeDiskDesiredSpec) bool {
	if labels == nil {
		return false
	}
	if labels[shared.LabelAutoReserve] != "true" {
		return false
	}
	if desired.State == v1alpha1.DesiredStateReserved && desired.Owner == "auto" {
		return false
	}
	return true
}
