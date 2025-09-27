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

package app

import (
	"fmt"

	"github.com/kubeflag/kubeflag/cmd/controller-manager/app/options"
	challengecontroller "github.com/kubeflag/kubeflag/pkg/controllers/challenge"
	instancecontroller "github.com/kubeflag/kubeflag/pkg/controllers/challengeinstance"
	datasynccontroller "github.com/kubeflag/kubeflag/pkg/controllers/datasyncer"
)

type controllerCreator func(*options.ControllerContext) error

// AllControllers stores the list of all controllers that we want to run,
// each entry holds the name of the controller and the corresponding
// start function that will essentially run the controller.
var AllControllers = map[string]controllerCreator{
	challengecontroller.ControllerName: createChallengeController,
	instancecontroller.ControllerName:  createInstanceController,
	datasynccontroller.ControllerName:  createDataSyncController,
}

func createAllControllers(ctrlCtx *options.ControllerContext) error {
	for name, create := range AllControllers {
		if err := create(ctrlCtx); err != nil {
			return fmt.Errorf("failed to create %q controller: %w", name, err)
		}
	}

	return nil
}

func createChallengeController(ctrlCtx *options.ControllerContext) error {
	return challengecontroller.Add(ctrlCtx.Ctx, ctrlCtx.Mgr, 1, ctrlCtx.Log)
}

func createInstanceController(ctrlCtx *options.ControllerContext) error {
	return instancecontroller.Add(ctrlCtx.Ctx, ctrlCtx.Mgr, 1, nil)
}

func createDataSyncController(ctrlCtx *options.ControllerContext) error {
	return datasynccontroller.Add(ctrlCtx.Ctx, ctrlCtx.Mgr, 1, ctrlCtx.Log)
}
