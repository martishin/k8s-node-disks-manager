package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type DesiredState string

const (
	DesiredStateAvailable DesiredState = "Available"
	DesiredStateReserved  DesiredState = "Reserved"
)

type NodePhase string

const (
	NodePhaseDiscovered NodePhase = "Discovered"
	NodePhaseAvailable  NodePhase = "Available"
	NodePhaseReserved   NodePhase = "Reserved"
	NodePhaseMissing    NodePhase = "Missing"
	NodePhaseError      NodePhase = "Error"
)

type NodeDiskDesiredSpec struct {
	// +kubebuilder:validation:Enum=Available;Reserved
	State DesiredState `json:"state,omitempty"`
	Owner string       `json:"owner,omitempty"`
}

type NodeDiskSpec struct {
	NodeName string              `json:"nodeName"`
	DiskID   string              `json:"diskID"`
	Path     string              `json:"path"`
	Desired  NodeDiskDesiredSpec `json:"desired,omitempty"`
}

type NodeStatus struct {
	Phase         NodePhase    `json:"phase,omitempty"`
	CapacityBytes int64        `json:"capacityBytes,omitempty"`
	LastSeenTime  *metav1.Time `json:"lastSeenTime,omitempty"`
	Message       string       `json:"message,omitempty"`
}

type ControllerStatus struct {
	Accepted           bool               `json:"accepted,omitempty"`
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}

type NodeDiskStatus struct {
	Node       NodeStatus       `json:"node,omitempty"`
	Controller ControllerStatus `json:"controller,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=ndisk
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.spec.nodeName`
// +kubebuilder:printcolumn:name="Disk",type=string,JSONPath=`.spec.diskID`
// +kubebuilder:printcolumn:name="Desired",type=string,JSONPath=`.spec.desired.state`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.node.phase`

type NodeDisk struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeDiskSpec   `json:"spec,omitempty"`
	Status NodeDiskStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

type NodeDiskList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeDisk `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodeDisk{}, &NodeDiskList{})
}
