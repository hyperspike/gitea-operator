/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// RunnerSpec defines the desired state of Runner
type RunnerSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// +kubebuilder:default:=1
	Replicas int `json:"replicas"`

	// +kubebuilder:default:=false
	Rootless bool `json:"rootless"`

	// The Gitea Org to create org runners in
	Org *OrgRef `json:"org,omitempty"`
	// the gitea instance to create global runners in
	Instance InstanceType `json:"instance,omitempty"`
}

// RunnerStatus defines the observed state of Runner
type RunnerStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// TLS detected at the instance deployment
	TLS bool `json:"tls,omitempty"`

	Provisioned bool `json:"provisioned"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Runner is the Schema for the runners API
// +kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=`.status.provisioned`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type Runner struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RunnerSpec   `json:"spec,omitempty"`
	Status RunnerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RunnerList contains a list of Runner
type RunnerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Runner `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Runner{}, &RunnerList{})
}
