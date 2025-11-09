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

package util

import (
	"crypto/x509"
	"errors"
	"fmt"
	"os"
)

var (
	// CABundle is set globally once by the main() function
	// and is used to overwrite the default set of CA certificates
	// loaded from the host system/pod.
	CaBundle *x509.CertPool
)

// SetCABundleFile reads a PEM-encoded file and replaces the current
// global CABundle with a new one. The file must contain at least one
// valid certificate.
func SetCABundleFile(filename string) error {
	content, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	CaBundle = x509.NewCertPool()
	if !CaBundle.AppendCertsFromPEM(content) {
		return errors.New("file does not contain valid PEM-encoded certificates")
	}

	return nil
}
