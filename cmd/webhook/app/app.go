package app

import (
	"context"
	"flag"

	"github.com/kubeflag/kubeflag/cmd/webhook/app/options"
	kubeflagv1 "github.com/kubeflag/kubeflag/pkg/api/v1alpha1"
	kubeflaglog "github.com/kubeflag/kubeflag/pkg/log"
	"github.com/kubeflag/kubeflag/pkg/util"
	"github.com/spf13/cobra"

	challengemutation "github.com/kubeflag/kubeflag/pkg/webhook/challenge/mutation"
	challengevalidation "github.com/kubeflag/kubeflag/pkg/webhook/challenge/validation"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	ctrlruntimelog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
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
		CertDir:  ".",
		CertName: opts.AdmissionTLSCertPath,
		KeyName:  opts.AdmissionTLSKeyPath,
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
		Cache: cache.Options{
			DefaultNamespaces: map[string]cache.Config{
				opts.Namespace: {},
			},
		},
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

	// mutation cannot, because we require separate defaulting for CREATE and UPDATE operations
	challengemutation.NewAdmissionHanlder(&log, mgr.GetScheme(), mgr.GetClient(), caPool).SetupWebhookWithManager(mgr)
	log.Info("Registered endpoints", "endpoints", mgr.GetWebhookServer())

	log.Info("Starting the webhook...")
	if err := mgr.Start(rootCtx); err != nil {
		log.Error(err, "The controller manager has failed")
		return err
	}
	return nil
}
