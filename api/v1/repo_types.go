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

// RepoSpec defines the desired state of Repo
type RepoSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// a brief explanation of the repo
	Description string `json:"description,omitempty"`
	// Whether the repository is private
	Private bool `json:"private,omitempty"`
	// Issue Label set to use
	IssueLabels string `json:"issue_labels,omitempty"`
	// Whether the repository should be auto-intialized?
	AutoInit bool `json:"auto_init,omitempty"`
	// Whether the repository is template
	Template bool `json:"template,omitempty"`
	// Gitignores to use
	Gitignores string `json:"gitignores,omitempty"`
	// License to use
	License string `json:"license,omitempty"`
	// Readme of the repository to create
	Readme string `json:"readme,omitempty"`
	// DefaultBranch of the repository (used when initializes and in template)
	DefaultBranch string `json:"default_branch,omitempty"`
	// TrustModel of the repository
	TrustModel string `json:"trust_model,omitempty"`

	User *UserRef `json:"user,omitempty"`
	Org  *OrgRef  `json:"org,omitempty"`
}

// RepoStatus defines the observed state of Repo
type RepoStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Provisioned bool `json:"provisioned"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Repo is the Schema for the repoes API
// +kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=`.status.provisioned`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type Repo struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RepoSpec   `json:"spec,omitempty"`
	Status RepoStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RepoList contains a list of Repo
type RepoList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Repo `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Repo{}, &RepoList{})
}
