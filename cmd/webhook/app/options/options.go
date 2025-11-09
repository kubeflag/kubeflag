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

package options

import (
	"flag"

	"github.com/kubeflag/kubeflag/pkg/log"
)

type WebhookServerRunOptions struct {
	AdmissionListenHost  string
	AdmissionListenPort  int
	AdmissionTLSCertPath string
	AdmissionTLSKeyPath  string
	CaBundleFile         string
	Namespace            string
	Kubeconfig           string
	MasterURL            string
	LogLevel             log.LogLevel
	LogFormat            log.Format
}

func (o *WebhookServerRunOptions) AddFlags(fs *flag.FlagSet) {
	if fs.Lookup("kubeconfig") == nil {
		fs.StringVar(&o.Kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	}
	if fs.Lookup("master") == nil {
		fs.StringVar(&o.MasterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	}
	fs.StringVar(&o.AdmissionListenHost, "listen-host", "", "The Host on which the Admission/Mutating Webhook will listen on")
	fs.IntVar(&o.AdmissionListenPort, "listen-port", 9876, "The port on which the Admission/Mutating Webhook will listen on")
	fs.StringVar(&o.AdmissionTLSCertPath, "tls-cert-path", "/tmp/cert/tls.crt", "The path of the TLS cert for the Admission/Mutating Webhook")
	fs.StringVar(&o.AdmissionTLSKeyPath, "tls-key-path", "/tmp/cert/tls.key", "The path of the TLS key for the Admission/Mutating Webhook")
	fs.StringVar(&o.CaBundleFile, "ca-bundle", "", "path to a file containing all PEM-encoded CA certificates (will be used instead of the host's certificates if set)")
	fs.StringVar(&o.Namespace, "namespace", "kubeflag", "The namespace where the webhooks will run")
	fs.Var(&o.LogFormat, "log-format", "Log format, one of [Console, Json]")
	fs.Var(&o.LogLevel, "log-debug", "Enables more verbose logging")
	o.Kubeconfig = fs.Lookup("kubeconfig").Value.(flag.Getter).Get().(string)
	o.MasterURL = fs.Lookup("master").Value.(flag.Getter).Get().(string)
}
