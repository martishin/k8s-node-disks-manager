package controller

import (
	"context"
	"fmt"
	"reflect"

	v1alpha1 "github.com/martishin/k8s-node-disks-manager/api/v1alpha1"
	"github.com/martishin/k8s-node-disks-manager/internal/controller/policy"
	"github.com/martishin/k8s-node-disks-manager/internal/shared"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const acceptedConditionType = "Accepted"

type NodeDiskReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=localtest.example.com,resources=nodedisks,verbs=get;list;watch;patch;update
// +kubebuilder:rbac:groups=localtest.example.com,resources=nodedisks/status,verbs=get;patch;update
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch;update

func (r *NodeDiskReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	var nd v1alpha1.NodeDisk
	if err := r.Get(ctx, req.NamespacedName, &nd); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	accepted := true
	reason := "Accepted"
	message := "NodeDisk object is valid"

	if nd.Spec.NodeName == "" || nd.Spec.DiskID == "" || nd.Spec.Path == "" {
		accepted = false
		reason = "InvalidSpec"
		message = "spec.nodeName, spec.diskID, and spec.path are required"
	}

	ctrlStatus := nd.Status.Controller
	ctrlStatus.Accepted = accepted
	ctrlStatus.ObservedGeneration = nd.Generation
	setCondition(&ctrlStatus.Conditions, metav1.Condition{
		Type:               acceptedConditionType,
		Status:             boolToConditionStatus(accepted),
		Reason:             reason,
		Message:            message,
		ObservedGeneration: nd.Generation,
		LastTransitionTime: metav1.Now(),
	})

	if !reflect.DeepEqual(nd.Status.Controller, ctrlStatus) {
		base := nd.DeepCopy()
		nd.Status.Controller = ctrlStatus
		if err := r.Status().Patch(ctx, &nd, client.MergeFrom(base)); err != nil {
			return ctrl.Result{}, err
		}
		if accepted {
			r.Recorder.Event(&nd, "Normal", "Accepted", message)
		} else {
			r.Recorder.Event(&nd, "Warning", "Rejected", message)
		}
	}

	if accepted {
		var latest v1alpha1.NodeDisk
		if err := r.Get(ctx, req.NamespacedName, &latest); err != nil {
			if apierrors.IsNotFound(err) {
				return ctrl.Result{}, nil
			}
			return ctrl.Result{}, err
		}
		nd = latest
	}

	if accepted && policy.ShouldAutoReserve(nd.Labels, nd.Spec.Desired) {
		base := nd.DeepCopy()
		nd.Spec.Desired.State = v1alpha1.DesiredStateReserved
		nd.Spec.Desired.Owner = "auto"
		if err := r.Patch(ctx, &nd, client.MergeFrom(base)); err != nil {
			return ctrl.Result{}, err
		}
		r.Recorder.Eventf(&nd, "Normal", "DesiredStateChanged", "set spec.desired.state=Reserved owner=auto")
		logger.Info("auto reserve policy applied", "name", nd.Name)
	} else if accepted && nd.Labels[shared.LabelAutoReserve] != "true" && nd.Spec.Desired.State == v1alpha1.DesiredStateReserved && nd.Spec.Desired.Owner == "auto" {
		base := nd.DeepCopy()
		nd.Spec.Desired.State = v1alpha1.DesiredStateAvailable
		nd.Spec.Desired.Owner = ""
		if err := r.Patch(ctx, &nd, client.MergeFrom(base)); err != nil {
			return ctrl.Result{}, err
		}
		r.Recorder.Eventf(&nd, "Normal", "DesiredStateChanged", "set spec.desired.state=Available owner=")
		logger.Info("auto reserve policy removed", "name", nd.Name)
	}

	return ctrl.Result{}, nil
}

func (r *NodeDiskReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("node-disks-operator")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.NodeDisk{}).
		Named("node-disk").
		Complete(r)
}

func setCondition(conditions *[]metav1.Condition, cond metav1.Condition) {
	idx := -1
	for i := range *conditions {
		if (*conditions)[i].Type == cond.Type {
			idx = i
			break
		}
	}
	if idx == -1 {
		*conditions = append(*conditions, cond)
		return
	}
	current := (*conditions)[idx]
	if current.Status != cond.Status || current.Reason != cond.Reason || current.Message != cond.Message {
		(*conditions)[idx] = cond
		return
	}
	(*conditions)[idx].ObservedGeneration = cond.ObservedGeneration
}

func boolToConditionStatus(v bool) metav1.ConditionStatus {
	if v {
		return metav1.ConditionTrue
	}
	return metav1.ConditionFalse
}

func ValidateNodeDiskSpec(nd *v1alpha1.NodeDisk) error {
	if nd.Spec.NodeName == "" || nd.Spec.DiskID == "" || nd.Spec.Path == "" {
		return fmt.Errorf("missing required fields in spec")
	}
	return nil
}
