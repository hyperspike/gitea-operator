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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// AuthSpec defines the desired state of Auth
type AuthSpec struct {
	// +kubebuilder:validation:Required
	Provider string `json:"provider"`

	// The client ID for the OIDC provider
	// +kubebuilder:validation:Required
	ClientID corev1.SecretKeySelector `json:"clientID"`
	// The client secret for the OIDC provider
	// +kubebuilder:validation:Required
	ClientSecret corev1.SecretKeySelector `json:"clientSecret"`

	// The URL to the OIDC provider (e.g. https://oidc.example.com)
	AutoDiscoveryURL string `json:"autoDiscoveryURL"`

	// Scopes to request from the OIDC provider
	Scopes []string `json:"scopes"`

	// Group Claim name to use for group membership
	GroupClaimName string `json:"groupClaimName"`

	// The Gitea instance to add the OIDC authentication to
	// +kubebuilder:validation:Required
	Instance InstanceType `json:"instance"`
}

// AuthStatus defines the observed state of Auth
type AuthStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Auth is the Schema for the auths API
type Auth struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AuthSpec   `json:"spec,omitempty"`
	Status AuthStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AuthList contains a list of Auth
type AuthList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Auth `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Auth{}, &AuthList{})
}
