/*
Copyright 2026 The KubeFlag Authors.

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

package app

import (
	"context"
	"flag"
	"path/filepath"

	"github.com/kubeflag/kubeflag/cmd/webhook/app/options"
	kubeflagv1 "github.com/kubeflag/kubeflag/pkg/api/v1alpha1"
	kubeflaglog "github.com/kubeflag/kubeflag/pkg/log"
	"github.com/kubeflag/kubeflag/pkg/util"
	"github.com/spf13/cobra"

	challengemutation "github.com/kubeflag/kubeflag/pkg/webhook/challenge/mutation"
	challengevalidation "github.com/kubeflag/kubeflag/pkg/webhook/challenge/validation"
	consumervalidation "github.com/kubeflag/kubeflag/pkg/webhook/consumer/validation"
	tenantmutation "github.com/kubeflag/kubeflag/pkg/webhook/tenant/mutation"
	tenantvalidation "github.com/kubeflag/kubeflag/pkg/webhook/tenant/validation"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	ctrlruntimelog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	ctrlruntimewebhook "sigs.k8s.io/controller-runtime/pkg/webhook"
)

const (
	webhookName = "kubeflag-webhook"
)

func NewWebhookCommand() *cobra.Command {
	opts := &options.WebhookServerRunOptions{}

	// Create a FlagSet and add your flags to it
	fs := flag.NewFlagSet(webhookName, flag.ExitOnError)
	opts.AddFlags(fs)

	// Create a Cobra command
	cmd := &cobra.Command{
		Use:   webhookName,
		Short: "Webhook Server for KubeFlag",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Parse the flags from the FlagSet
			err := fs.Parse(args)
			if err != nil {
				return err
			}
			return runWebhookManager(opts)
		},
	}

	// Add the FlagSet to the Cobra command
	cmd.Flags().AddGoFlagSet(fs)

	return cmd
}

func runWebhookManager(opts *options.WebhookServerRunOptions) error {
	// Initialize logger
	rootCtx := signals.SetupSignalHandler()
	rawLog, _ := kubeflaglog.NewZapLogger(opts.LogLevel, opts.LogFormat)
	log := rawLog.WithName(webhookName)
	ctrlruntimelog.SetLogger(log)
	// Setting up kubernetes Configuration
	cfg, err := ctrlruntime.GetConfig()
	if err != nil {
		log.Error(err, "Failed to build kubeconfig")
		return err
	}

	webhookOptions := ctrlruntimewebhook.Options{
		CertDir:  filepath.Dir(opts.AdmissionTLSCertPath),
		CertName: filepath.Base(opts.AdmissionTLSCertPath),
		KeyName:  filepath.Base(opts.AdmissionTLSKeyPath),
		Host:     opts.AdmissionListenHost,
		Port:     opts.AdmissionListenPort,
	}

	caBundle, err := util.NewCABundleFromFile(opts.CaBundleFile)
	if err != nil {
		log.Error(err, "Failed to create new CABundle")
		return err
	}
	caPool := caBundle.CertPool()

	mgr, err := manager.New(cfg, manager.Options{
		BaseContext: func() context.Context {
			return rootCtx
		},
		Metrics:       metricsserver.Options{BindAddress: "0"}, // disabled for webhook-only binary
		WebhookServer: ctrlruntimewebhook.NewServer(webhookOptions),
	})

	if err != nil {
		log.Error(err, "Failed to create the manager")
		return err
	}

	if err := kubeflagv1.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "Failed to register scheme")
		return err
	}

	// validation webhook can already use ctrl-runtime boilerplate
	if err := challengevalidation.Add(mgr, log, caPool); err != nil {
		log.Error(err, "Failed to setup challenge validation webhook")
		return err
	}

	// consumer validation webhook
	if err := consumervalidation.Add(mgr, log); err != nil {
		log.Error(err, "Failed to setup consumer validation webhook")
		return err
	}

	// tenant validation webhook
	if err := tenantvalidation.Add(mgr, log); err != nil {
		log.Error(err, "Failed to setup tenant validation webhook")
		return err
	}

	// mutation cannot, because we require separate defaulting for CREATE and UPDATE operations
	challengemutation.NewAdmissionHanlder(&log, mgr.GetScheme(), mgr.GetClient(), caPool).SetupWebhookWithManager(mgr)

	// tenant mutation webhook (CREATE-only defaulting)
	tenantmutation.NewAdmissionHandler(&log, mgr.GetScheme()).SetupWebhookWithManager(mgr)
	log.Info("Registered endpoints", "endpoints", mgr.GetWebhookServer())

	log.Info("Starting the webhook...")
	if err := mgr.Start(rootCtx); err != nil {
		log.Error(err, "The controller manager has failed")
		return err
	}
	return nil
}
