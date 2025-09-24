/*
Copyright 2024 The KubeFlag contributors.

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
	"context"
	"flag"

	"github.com/go-logr/logr"
	"github.com/mohamed-rafraf/kubeflag/pkg/log"

	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type ControllerManagerRunOptions struct {
	EnableLeaderElection bool
	Port                 int
	MetricsBindAddress   string
	LogLevel             log.LogLevel
	LogFormat            log.Format
	WorkerName           string
}

type ControllerContext struct {
	Ctx        context.Context
	RunOptions *ControllerManagerRunOptions
	Mgr        manager.Manager
	Log        *logr.Logger
}

func (o *ControllerManagerRunOptions) AddFlags(fs *flag.FlagSet) {
	fs.BoolVar(&o.EnableLeaderElection, "leader-elect", false, "Enable leader election for controller manager.")
	fs.Var(&o.LogLevel, "log-level", "Enables more verbose logging")
	fs.IntVar(&o.Port, "port", 9443, "The port the controller-manager's webhook server binds to.")
	fs.StringVar(&o.MetricsBindAddress, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	fs.StringVar(&o.WorkerName, "worker-name", "", "The name of the worker that will only processes resources with label=worker-name.")
	fs.Var(&o.LogFormat, "log-format", "Log format, one of [Console, Json]")
}
