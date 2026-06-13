package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TunnelProtocol enumerates the supported wire protocols.
// +kubebuilder:validation:Enum=http;tcp
type TunnelProtocol string

const (
	TunnelProtocolHTTP TunnelProtocol = "http"
	TunnelProtocolTCP  TunnelProtocol = "tcp"
)

// TunnelPhase tracks the high-level lifecycle of a tunnel.
// +kubebuilder:validation:Enum=Pending;Assigned;Active;Failed;Terminating
type TunnelPhase string

const (
	TunnelPhasePending     TunnelPhase = "Pending"
	TunnelPhaseAssigned    TunnelPhase = "Assigned"
	TunnelPhaseActive      TunnelPhase = "Active"
	TunnelPhaseFailed      TunnelPhase = "Failed"
	TunnelPhaseTerminating TunnelPhase = "Terminating"
)

// TunnelSpec describes the desired state of a tunnel. Spec is normally
// authored by a controller (in response to an agent's TunnelRequest)
// rather than a human, but it is a valid first-class resource.
type TunnelSpec struct {
	// Protocol selects between HTTP (subdomain-routed) and raw TCP.
	Protocol TunnelProtocol `json:"protocol"`

	// Hostname is the public hostname assigned to this tunnel
	// (e.g. `alice.xpos-io.com`).
	// +kubebuilder:validation:MinLength=1
	Hostname string `json:"hostname"`

	// AgentRef points at the Agent CR that requested this tunnel.
	AgentRef AgentReference `json:"agentRef"`
}

// AgentReference is a namespace/name pointer to an Agent CR.
type AgentReference struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

// TunnelStatus carries observed state.
type TunnelStatus struct {
	// Phase is a coarse-grained lifecycle state for human consumption.
	// +optional
	Phase TunnelPhase `json:"phase,omitempty"`

	// AssignedPod is the data-plane pod currently serving this
	// tunnel's bridged byte streams. Mirrors the AgentRef's relay
	// pod once placement is complete.
	// +optional
	AssignedPod *RelayPodRef `json:"assignedPod,omitempty"`

	// PublicAddr is the external address advertised to end users.
	// +optional
	PublicAddr string `json:"publicAddr,omitempty"`

	// PrivateAddr is the address the agent dials back on for new
	// visitor connections.
	//
	// Deprecated as of protocol v2 (yamux multiplex): the agent no
	// longer opens a private listener. Retained on the wire for
	// backwards compatibility with tooling that read v1 status.
	// +optional
	PrivateAddr string `json:"privateAddr,omitempty"`

	// TCPPort is the public TCP port allocated to this tunnel by
	// the operator's port allocator. Populated only for TCP
	// tunnels (spec.protocol=tcp). The value matches the listener
	// port on the parent Gateway used by the reconciled TCPRoute.
	//
	// Authoritative: this field is set by the operator. The relay
	// reads it back (polling the CR during tunnel handshake) and
	// binds its public listener to exactly this port so the
	// reconciled TCPRoute and the listening relay agree.
	// +optional
	TCPPort *int32 `json:"tcpPort,omitempty"`

	// ObservedGeneration is the .metadata.generation last reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions follow the standard k8s conditions pattern.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=xtun
// +kubebuilder:printcolumn:name="Protocol",type=string,JSONPath=`.spec.protocol`
// +kubebuilder:printcolumn:name="Hostname",type=string,JSONPath=`.spec.hostname`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Public",type=string,JSONPath=`.status.publicAddr`
// +kubebuilder:printcolumn:name="TCPPort",type=integer,JSONPath=`.status.tcpPort`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Tunnel is a single forward of public traffic to a connected agent.
type Tunnel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TunnelSpec   `json:"spec,omitempty"`
	Status TunnelStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TunnelList contains a list of Tunnel.
type TunnelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Tunnel `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Tunnel{}, &TunnelList{})
}
