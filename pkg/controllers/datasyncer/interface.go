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

package datasyncer

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type SyncableObject interface {
	ctrlruntimeclient.Object
	GetData() map[string][]byte
	SetData(map[string][]byte)
	GetTypeMeta() metav1.TypeMeta
	GetBaseObject() ctrlruntimeclient.Object
}

type SecretWrapper struct {
	*corev1.Secret
}

func (s SecretWrapper) GetData() map[string][]byte {
	return s.Data
}
func (s SecretWrapper) SetData(d map[string][]byte) {
	s.Data = d
}
func (s SecretWrapper) GetTypeMeta() metav1.TypeMeta {
	return s.TypeMeta
}

type ConfigMapWrapper struct {
	*corev1.ConfigMap
}

func (s SecretWrapper) GetBaseObject() ctrlruntimeclient.Object    { return s.Secret }
func (c ConfigMapWrapper) GetBaseObject() ctrlruntimeclient.Object { return c.ConfigMap }

func (c ConfigMapWrapper) GetData() map[string][]byte {
	result := make(map[string][]byte)

	for k, v := range c.Data {
		result[k] = []byte(v)
	}

	for k, v := range c.BinaryData {
		result[k] = v
	}

	return result
}
func (c ConfigMapWrapper) SetData(d map[string][]byte) {
	// Convert []byte -> string
	cmData := make(map[string]string, len(d))
	for k, v := range d {
		cmData[k] = string(v)
	}
	c.Data = cmData
	// c.BinaryData = d
}
func (c ConfigMapWrapper) GetTypeMeta() metav1.TypeMeta {
	return c.TypeMeta
}
