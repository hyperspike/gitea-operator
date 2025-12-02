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

// MigrateRepoSpec defines the desired state of MigrateRepo
type MigrateRepoSpec struct {
	User *UserRef `json:"user,omitempty"`
	Org  *OrgRef  `json:"org,omitempty"`
	// URL is the source URL of the repository to migrate.
	URL string `json:"url"`
	// Description is the description of the repository.
	Description string `json:"description,omitempty"`
	// Private indicates whether the repository is private.
	Private bool `json:"private,omitempty"`
	// Service The service to pull from
	// +kubebuilder:validation:Enum=git;github;gitlab;gogs;gitea
	Service string `json:"service,omitempty"`
	// Mirror indicates whether to mirror the repository.
	Mirror bool `json:"mirror,omitempty"`
}

// MigrateRepoStatus defines the observed state of MigrateRepo.
type MigrateRepoStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the MigrateRepo resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// MigrateRepo is the Schema for the migraterepoes API
type MigrateRepo struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of MigrateRepo
	// +required
	Spec MigrateRepoSpec `json:"spec"`

	// status defines the observed state of MigrateRepo
	// +optional
	Status MigrateRepoStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// MigrateRepoList contains a list of MigrateRepo
type MigrateRepoList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MigrateRepo `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MigrateRepo{}, &MigrateRepoList{})
}
