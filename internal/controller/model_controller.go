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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	ollamav1 "github.com/StartUpNationLabs/simple-ollama-operator/api/v1"
	"os"
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
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	// Read url from the Model instance
	ollamaUrl := os.Getenv("OLLAMA_URL")
	if ollamaUrl == "" {
		logger.Error(err, "unable to fetch Ollama URL")
		panic("OLLAMA_URL not set")
	}

	// create a new Ollama Client
	ollamaClient, err := ollama_client.NewClientWithResponses(ollamaUrl)
	if err != nil {
		logger.Error(err, "unable to create Ollama Client")
		return ctrl.Result{}, err
	}
	logger.Info("Ollama Client created")
	// If the Model is being deleted, delete it from the Ollama Client
	modelFinalizer := "model.finalizer.ollama.ollama.startupnation"

	if model.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object.
		if !containsString(model.ObjectMeta.Finalizers, modelFinalizer) {
			model.ObjectMeta.Finalizers = append(model.ObjectMeta.Finalizers, modelFinalizer)
			if err := r.Update(context.Background(), model); err != nil {
				return reconcile.Result{}, err
			}
		}
	} else {
		// The object is being deleted
		if containsString(model.ObjectMeta.Finalizers, modelFinalizer) {
			// our finalizer is present, so lets handle our external dependency
			// first, we delete the external dependency
			logger.Info("Deleting Model", "Model Name", model.Spec.ModelName, "Ollama URL", ollamaUrl)
			_, err := ollamaClient.DeleteApiDelete(ctx, &ollama_client.DeleteApiDeleteParams{
				Model: model.Spec.ModelName,
			})
			if err != nil {
				logger.Error(err, "unable to delete Model")
			}

			// remove our finalizer from the list and update it.
			model.ObjectMeta.Finalizers = removeString(model.ObjectMeta.Finalizers, modelFinalizer)
			if err := r.Update(context.Background(), model); err != nil {
				return reconcile.Result{}, err
			}
		}

		// Our finalizer has finished, so the reconciler can do nothing.
		return reconcile.Result{}, nil
	}
	// if the Model is not being deleted, start reconciliation

	// get the model from the Ollama Client
	logger.Info("Checking if Model exists", "Model Name", model.Spec.ModelName, "Ollama URL", ollamaUrl)
	res, err := ollamaClient.PostApiShowWithResponse(ctx, ollama_client.PostApiShowJSONRequestBody{
		Name: &model.Spec.ModelName,
	})
	if err == nil && res.Status() == "200" {
		logger.Info("Model exists", "Model Name", model.Spec.ModelName, "Ollama URL", ollamaUrl)
		if res.JSON200 != nil {
			logger.Info("Model exists", "Model Name", res.JSON200.Parameters, "Ollama URL", ollamaUrl)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	// if the model does not exist, create it
	logger.Info("Model does not exist, creating Model", "Model Name", model.Spec.ModelName, "Ollama URL", ollamaUrl)
	stream := false
	_, err = ollamaClient.PostApiPull(ctx, ollama_client.PostApiPullJSONRequestBody{
		Name:   &model.Spec.ModelName,
		Stream: &stream,
	})
	if err != nil {
		logger.Error(err, "unable to create Model")
		return ctrl.Result{}, err
	}
	logger.Info("Model created", "Model Name", model.Spec.ModelName, "Ollama URL", ollamaUrl)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ModelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ollamav1.Model{}).
		Complete(r)
}
