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

// OrgSpec defines the desired state of Org
type OrgSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// The human readable name of the organization.
	FullName string `json:"fullname,omitempty"`

	// A brief explanation of the organization.
	Description string `json:"description,omitempty"`

	// The url of the website associated with this org.
	Website string `json:"website,omitempty"`

	// The physical location of the org, if any
	Location string `json:"location,omitempty"`

	// users list to
	Teams []Team `json:"users,omitempty"`

	// The ACL name of the org. public, private, etc.
	Visibility string `json:"visibility,omitempty"`
	// the gitea instance to build the org in
	Instance InstanceType `json:"instance"`
}

// OrgStatus defines the observed state of Org
type OrgStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Provisioned bool `json:"provisioned"`
}

type Team struct {
	Name            string   `json:"name"`
	Description     string   `json:"description,omitempty"`
	Permission      string   `json:"permission,omitempty"`
	CreateOrgRepo   bool     `json:"createOrgRepo,omitempty"`
	IncludeAllRepos bool     `json:"includeAllRepos,omitempty"`
	Members         []string `json:"members,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Org is the Schema for the orgs API
// +kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=`.status.provisioned`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type Org struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OrgSpec   `json:"spec,omitempty"`
	Status OrgStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OrgList contains a list of Org
type OrgList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Org `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Org{}, &OrgList{})
}
