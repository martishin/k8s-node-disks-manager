package controller

import (
	"context"
	"testing"

	v1alpha1 "github.com/martishin/k8s-node-disks-manager/api/v1alpha1"
	"github.com/martishin/k8s-node-disks-manager/internal/shared"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReconcileRejectsInvalidSpecWithoutTouchingNodeStatus(t *testing.T) {
	nd := &v1alpha1.NodeDisk{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-disk-node-a-disk1",
		},
		Spec: v1alpha1.NodeDiskSpec{
			DiskID: "disk1",
			Path:   "/var/lib/node-disks-manager/disks/disk1.img",
		},
		Status: v1alpha1.NodeDiskStatus{
			Node: v1alpha1.NodeStatus{
				Phase:   v1alpha1.NodePhaseReserved,
				Message: "existing-node-state",
			},
		},
	}

	reconciler, c := newTestReconciler(t, nd)
	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: nd.Name}}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	var got v1alpha1.NodeDisk
	if err := c.Get(context.Background(), types.NamespacedName{Name: nd.Name}, &got); err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if got.Status.Controller.Accepted {
		t.Fatal("status.controller.accepted should be false for invalid spec")
	}
	accepted := findCondition(t, got.Status.Controller.Conditions, acceptedConditionType)
	if accepted.Status != metav1.ConditionFalse {
		t.Fatalf("Accepted condition status = %q, want %q", accepted.Status, metav1.ConditionFalse)
	}
	if accepted.Reason != "InvalidSpec" {
		t.Fatalf("Accepted condition reason = %q, want %q", accepted.Reason, "InvalidSpec")
	}

	if got.Status.Node.Phase != v1alpha1.NodePhaseReserved {
		t.Fatalf("status.node.phase = %q, want %q", got.Status.Node.Phase, v1alpha1.NodePhaseReserved)
	}
	if got.Status.Node.Message != "existing-node-state" {
		t.Fatalf("status.node.message = %q, want %q", got.Status.Node.Message, "existing-node-state")
	}
}

func TestReconcileAutoReservesWhenLabelEnabled(t *testing.T) {
	nd := &v1alpha1.NodeDisk{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-disk-node-a-disk2",
			Labels: map[string]string{
				shared.LabelAutoReserve: "true",
			},
		},
		Spec: v1alpha1.NodeDiskSpec{
			NodeName: "node-a",
			DiskID:   "disk2",
			Path:     "/var/lib/node-disks-manager/disks/disk2.img",
			Desired: v1alpha1.NodeDiskDesiredSpec{
				State: v1alpha1.DesiredStateAvailable,
			},
		},
		Status: v1alpha1.NodeDiskStatus{
			Node: v1alpha1.NodeStatus{
				Phase: v1alpha1.NodePhaseAvailable,
			},
		},
	}

	reconciler, c := newTestReconciler(t, nd)
	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: nd.Name}}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	var got v1alpha1.NodeDisk
	if err := c.Get(context.Background(), types.NamespacedName{Name: nd.Name}, &got); err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if got.Spec.Desired.State != v1alpha1.DesiredStateReserved {
		t.Fatalf("spec.desired.state = %q, want %q", got.Spec.Desired.State, v1alpha1.DesiredStateReserved)
	}
	if got.Spec.Desired.Owner != "auto" {
		t.Fatalf("spec.desired.owner = %q, want %q", got.Spec.Desired.Owner, "auto")
	}
	if !got.Status.Controller.Accepted {
		t.Fatal("status.controller.accepted should be true for valid spec")
	}
	accepted := findCondition(t, got.Status.Controller.Conditions, acceptedConditionType)
	if accepted.Status != metav1.ConditionTrue {
		t.Fatalf("Accepted condition status = %q, want %q", accepted.Status, metav1.ConditionTrue)
	}
	if got.Status.Node.Phase != v1alpha1.NodePhaseAvailable {
		t.Fatalf("status.node.phase = %q, want %q", got.Status.Node.Phase, v1alpha1.NodePhaseAvailable)
	}
}

func TestReconcileReleasesAutoOwnedReservationWhenLabelRemoved(t *testing.T) {
	nd := &v1alpha1.NodeDisk{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-disk-node-a-disk3",
		},
		Spec: v1alpha1.NodeDiskSpec{
			NodeName: "node-a",
			DiskID:   "disk3",
			Path:     "/var/lib/node-disks-manager/disks/disk3.img",
			Desired: v1alpha1.NodeDiskDesiredSpec{
				State: v1alpha1.DesiredStateReserved,
				Owner: "auto",
			},
		},
		Status: v1alpha1.NodeDiskStatus{
			Node: v1alpha1.NodeStatus{
				Phase: v1alpha1.NodePhaseReserved,
			},
		},
	}

	reconciler, c := newTestReconciler(t, nd)
	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: nd.Name}}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	var got v1alpha1.NodeDisk
	if err := c.Get(context.Background(), types.NamespacedName{Name: nd.Name}, &got); err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if got.Spec.Desired.State != v1alpha1.DesiredStateAvailable {
		t.Fatalf("spec.desired.state = %q, want %q", got.Spec.Desired.State, v1alpha1.DesiredStateAvailable)
	}
	if got.Spec.Desired.Owner != "" {
		t.Fatalf("spec.desired.owner = %q, want empty", got.Spec.Desired.Owner)
	}
	if !got.Status.Controller.Accepted {
		t.Fatal("status.controller.accepted should be true for valid spec")
	}
	accepted := findCondition(t, got.Status.Controller.Conditions, acceptedConditionType)
	if accepted.Status != metav1.ConditionTrue {
		t.Fatalf("Accepted condition status = %q, want %q", accepted.Status, metav1.ConditionTrue)
	}
	if got.Status.Node.Phase != v1alpha1.NodePhaseReserved {
		t.Fatalf("status.node.phase = %q, want %q", got.Status.Node.Phase, v1alpha1.NodePhaseReserved)
	}
}

func newTestReconciler(t *testing.T, objects ...client.Object) (*NodeDiskReconciler, client.Client) {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.NodeDisk{}).
		WithObjects(objects...).
		Build()

	return &NodeDiskReconciler{
		Client:   c,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(32),
	}, c
}

func findCondition(t *testing.T, conditions []metav1.Condition, condType string) metav1.Condition {
	t.Helper()
	for _, cond := range conditions {
		if cond.Type == condType {
			return cond
		}
	}
	t.Fatalf("condition %q not found", condType)
	return metav1.Condition{}
}
