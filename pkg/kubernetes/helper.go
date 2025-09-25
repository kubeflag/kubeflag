/*
Copyright 2025 The KubeFlag Authors.

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
package kubernetes

import (
	"fmt"
	"regexp"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/sets"
)

var tokenValidator = regexp.MustCompile(`[bcdfghjklmnpqrstvwxz2456789]{6}\.[bcdfghjklmnpqrstvwxz2456789]{16}`)

// HasFinalizer tells if a object has all the given finalizers.
func HasFinalizer(o metav1.Object, names ...string) bool {
	return sets.New(o.GetFinalizers()...).HasAll(names...)
}

func HasAnyFinalizer(o metav1.Object, names ...string) bool {
	return sets.New(o.GetFinalizers()...).HasAny(names...)
}

// RemoveFinalizer removes the given finalizers from the object.
func RemoveFinalizer(obj metav1.Object, toRemove ...string) {
	set := sets.New(obj.GetFinalizers()...)
	set.Delete(toRemove...)
	obj.SetFinalizers(sets.List(set))
}

// AddFinalizer will add the given finalizer to the object. It uses a StringSet to avoid duplicates.
func AddFinalizer(obj metav1.Object, finalizers ...string) {
	set := sets.New(obj.GetFinalizers()...)
	set.Insert(finalizers...)
	obj.SetFinalizers(sets.List(set))
}

// GenerateToken generates a new, random token that can be used
// as an admin and kubelet token.
func GenerateToken() string {
	return fmt.Sprintf("%s.%s", rand.String(6), rand.String(16))
}

// ValidateKubernetesToken checks if a given token is syntactically correct.
func ValidateKubernetesToken(token string) error {
	if !tokenValidator.MatchString(token) {
		return fmt.Errorf("token is malformed, must match %s", tokenValidator.String())
	}

	return nil
}

func HasOwnerReference(o metav1.Object, ref metav1.OwnerReference) bool {
	for _, r := range o.GetOwnerReferences() {
		if equalOwnerRefs(r, ref) {
			return true
		}
	}

	return false
}

func equalOwnerRefKinds(a, b metav1.OwnerReference) bool {
	return a.APIVersion == b.APIVersion && a.Kind == b.Kind
}

func equalOwnerRefs(a, b metav1.OwnerReference) bool {
	return equalOwnerRefKinds(a, b) && a.Name == b.Name
}

func EnsureAnnotations(o metav1.Object, toEnsure map[string]string) {
	annotations := o.GetAnnotations()

	if annotations == nil {
		annotations = make(map[string]string)
	}
	for key, value := range toEnsure {
		annotations[key] = value
	}
	o.SetAnnotations(annotations)
}
