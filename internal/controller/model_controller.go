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

package controller

import (
	"context"
	"github.com/StartUpNationLabs/simple-ollama-operator/internal/ollama_client"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	ollamav1 "github.com/StartUpNationLabs/simple-ollama-operator/api/v1"
)

// ModelReconciler reconciles a Model object
type ModelReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=ollama.ollama.startupnation,resources=models,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ollama.ollama.startupnation,resources=models/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ollama.ollama.startupnation,resources=models/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Model object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *ModelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the Model instance
	model := &ollamav1.Model{}
	err := r.Get(ctx, req.NamespacedName, model)
	if err != nil {
		logger.Error(err, "unable to fetch Model")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// create a new Ollama Client
	ollamaClient, err := ollama_client.NewClientWithResponses(model.Spec.OllamaUrl)
	if err != nil {
		logger.Error(err, "unable to create Ollama Client")
		return ctrl.Result{}, err
	}

	// If the Model is being deleted, delete it from the Ollama Client
	if model.DeletionTimestamp != nil {
		_, err = ollamaClient.DeleteApiDelete(ctx, &ollama_client.DeleteApiDeleteParams{
			Model: model.Spec.ModelName,
		})
		if err != nil {
			logger.Error(err, "unable to delete Model")
		}
		return ctrl.Result{}, nil
	}
	// if the Model is not being deleted, start reconciliation

	// get the model from the Ollama Client
	res, err := ollamaClient.PostApiShowWithResponse(ctx, ollama_client.PostApiShowJSONRequestBody{
		Name: &model.Spec.ModelName,
	})
	if err == nil {
		if res.JSON200.Modelfile != nil {
			logger.Info(*res.JSON200.Modelfile, "Model already exists")
		}
		return ctrl.Result{}, err
	}
	// if the model does not exist, create it
	stream := false
	_, err = ollamaClient.PostApiPull(ctx, ollama_client.PostApiPullJSONRequestBody{
		Name:   &model.Spec.ModelName,
		Stream: &stream,
	})
	if err != nil {
		logger.Error(err, "unable to create Model")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ModelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ollamav1.Model{}).
		Complete(r)
}
