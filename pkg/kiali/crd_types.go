package kiali

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// KialiCR mirrors kiali.io/v1alpha1 fields used for in-cluster URL discovery.
type KialiCR struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KialiSpec   `json:"spec,omitempty"`
	Status KialiStatus `json:"status,omitempty"`
}

type KialiSpec struct {
	Deployment KialiDeploymentSpec `json:"deployment,omitempty"`
	Server     KialiServerSpec     `json:"server,omitempty"`
}

type KialiDeploymentSpec struct {
	InstanceName string `json:"instance_name,omitempty"`
	Namespace    string `json:"namespace,omitempty"`
}

type KialiServerSpec struct {
	Port    int64  `json:"port,omitempty"`
	WebRoot string `json:"web_root,omitempty"`
}

type KialiStatus struct {
	Deployment KialiDeploymentStatus `json:"deployment,omitempty"`
}

type KialiDeploymentStatus struct {
	InstanceName string `json:"instanceName,omitempty"`
	Namespace    string `json:"namespace,omitempty"`
}
