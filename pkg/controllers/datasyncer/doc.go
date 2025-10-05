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

// Package datasyncer implements a Kubernetes controller that automatically synchronizes Secrets and ConfigMaps across namespaces.
// It watches for source objects annotated with target namespaces, creates or updates copies of these objects in each target namespace,
// and ensures they remain consistent with the source. Managed copies are labeled and finalized for proper cleanup,
// allowing the controller to detect deletions or annotation changes and remove outdated copies automatically.
// This provides a simple, Kubernetes-native way to replicate configuration data across environments while ensuring consistency,
// traceability, and automated lifecycle management.

package datasyncer
