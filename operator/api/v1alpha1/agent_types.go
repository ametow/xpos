package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AgentSpec describes a connected agent session as observed by a relay
// (data-plane) pod. The relay creates one Agent per active control
// connection and deletes it when the agent disconnects. A separate
// Lease (coordination.k8s.io) is used as the heartbeat; the operator
// garbage collects Agents whose corresponding relay Lease has expired.
type AgentSpec struct {
	// Identity is the authenticated identity of the agent (e.g. the
	// GitHub login it presented). Used to derive its tunnel
	// hostname.
	// +kubebuilder:validation:MinLength=1
	Identity string `json:"identity"`

	// SessionID uniquely identifies this control-channel session.
	// Regenerated on every reconnect so we can disambiguate agents
	// that share an Identity.
	// +kubebuilder:validation:MinLength=1
	SessionID string `json:"sessionID"`

	// RelayPod is the data-plane pod that currently owns the
	// control connection.
	RelayPod RelayPodRef `json:"relayPod"`
}

// RelayPodRef points at a specific data-plane Pod by namespace/name.
type RelayPodRef struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

// AgentStatus carries observed state.
type AgentStatus struct {
	// LastHeartbeat mirrors the renewTime of the relay's Lease. The
	// controller updates this field opportunistically; the
	// authoritative heartbeat is the Lease itself.
	// +optional
	LastHeartbeat *metav1.Time `json:"lastHeartbeat,omitempty"`

	// Conditions follow the standard k8s conditions pattern.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=xagent
// +kubebuilder:printcolumn:name="Identity",type=string,JSONPath=`.spec.identity`
// +kubebuilder:printcolumn:name="Relay",type=string,JSONPath=`.spec.relayPod.name`
// +kubebuilder:printcolumn:name="LastSeen",type=date,JSONPath=`.status.lastHeartbeat`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Agent is an active xpos agent session.
type Agent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentSpec   `json:"spec,omitempty"`
	Status AgentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AgentList contains a list of Agent.
type AgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Agent `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Agent{}, &AgentList{})
}
