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
	"os"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	ollamav1 "github.com/StartUpNationLabs/simple-ollama-operator/api/v1"
)

// CustomModelReconciler reconciles a CustomModel object
type CustomModelReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=ollama.ollama.startupnation,resources=custommodels,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ollama.ollama.startupnation,resources=custommodels/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ollama.ollama.startupnation,resources=custommodels/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the CustomModel object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *CustomModelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the Model instance
	CustomModel := &ollamav1.CustomModel{}
	err := r.Get(ctx, req.NamespacedName, CustomModel)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	// Read url from the CustomModel instance
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
	// If the CustomModel is being deleted, delete it from the Ollama Client
	CustomModelFinalizer := "CustomModel.finalizer.ollama.ollama.startupnation"

	if CustomModel.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object.
		if !containsString(CustomModel.ObjectMeta.Finalizers, CustomModelFinalizer) {
			CustomModel.ObjectMeta.Finalizers = append(CustomModel.ObjectMeta.Finalizers, CustomModelFinalizer)
			if err := r.Update(context.Background(), CustomModel); err != nil {
				return reconcile.Result{}, err
			}
		}
	} else {
		// The object is being deleted
		if containsString(CustomModel.ObjectMeta.Finalizers, CustomModelFinalizer) {
			// our finalizer is present, so lets handle our external dependency
			// first, we delete the external dependency
			logger.Info("Deleting CustomModel", "CustomModel Name", CustomModel.Spec.ModelName, "Ollama URL", ollamaUrl)
			_, err := ollamaClient.DeleteApiDelete(ctx, &ollama_client.DeleteApiDeleteParams{
				Model: CustomModel.Spec.ModelName,
			})
			if err != nil {
				logger.Error(err, "unable to delete CustomModel")
			}

			// remove our finalizer from the list and update it.
			CustomModel.ObjectMeta.Finalizers = removeString(CustomModel.ObjectMeta.Finalizers, CustomModelFinalizer)
			if err := r.Update(context.Background(), CustomModel); err != nil {
				return reconcile.Result{}, err
			}
		}

		// Our finalizer has finished, so the reconciler can do nothing.
		return reconcile.Result{}, nil
	}
	// if the CustomModel is not being deleted, start reconciliation

	// get the CustomModel from the Ollama Client
	logger.Info("Checking if CustomModel exists", "CustomModel Name", CustomModel.Spec.ModelName, "Ollama URL", ollamaUrl)
	res, err := ollamaClient.PostApiShowWithResponse(ctx, ollama_client.PostApiShowJSONRequestBody{
		Name: &CustomModel.Spec.ModelName,
	})
	logger.Info("Checking if CustomModel exists", "CustomModel Name", CustomModel.Spec.ModelName, "Ollama URL", ollamaUrl, "status", res.Status())
	if err == nil && res.StatusCode() == 200 {
		logger.Info("CustomModel exists", "CustomModel Name", CustomModel.Spec.ModelName, "Ollama URL", ollamaUrl)
		if res.JSON200 != nil {
			logger.Info("Checking if the ModelFile is the same", "CustomModel Name", CustomModel.Spec.ModelName, "Ollama URL", ollamaUrl)
			if *res.JSON200.Parameters == CustomModel.Spec.ModelFile {
				logger.Info("ModelFile is the same", "CustomModel Name", CustomModel.Spec.ModelName, "Ollama URL", ollamaUrl)
				return ctrl.Result{}, nil
			}
			logger.Info("ModelFile is not the same", "CustomModel Name", CustomModel.Spec.ModelName, "Ollama URL", ollamaUrl)
			// delete the CustomModel and create a new one
			logger.Info("Deleting CustomModel", "CustomModel Name", CustomModel.Spec.ModelName, "Ollama URL", ollamaUrl)
			_, err := ollamaClient.DeleteApiDelete(ctx, &ollama_client.DeleteApiDeleteParams{
				Model: CustomModel.Spec.ModelName,
			})
			if err != nil {
				logger.Error(err, "unable to delete CustomModel")
			}
			// Create the CustomModel
			logger.Info("Creating CustomModel", "CustomModel Name", CustomModel.Spec.ModelName, "Ollama URL", ollamaUrl)
			stream := false
			_, err = ollamaClient.PostApiCreate(ctx, ollama_client.PostApiCreateJSONRequestBody{
				Name:      &CustomModel.Spec.ModelName,
				Modelfile: &CustomModel.Spec.ModelFile,
				Stream:    &stream,
			})
			if err != nil {
				logger.Error(err, "unable to create CustomModel")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, err
	}
	// if the CustomModel does not exist, create it
	logger.Info("CustomModel does not exist, creating CustomModel", "CustomModel Name", CustomModel.Spec.ModelName, "Ollama URL", ollamaUrl, "ModelFile", CustomModel.Spec.ModelFile)
	stream := false
	_, err = ollamaClient.PostApiCreate(ctx, ollama_client.PostApiCreateJSONRequestBody{
		Name:      &CustomModel.Spec.ModelName,
		Modelfile: &CustomModel.Spec.ModelFile,
		Stream:    &stream,
	})
	if err != nil || res.StatusCode() != 200 {
		logger.Error(err, "unable to create CustomModel")
		return ctrl.Result{}, err
	}
	logger.Info("CustomModel created", "CustomModel Name", CustomModel.Spec.ModelName, "Ollama URL", ollamaUrl)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CustomModelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ollamav1.CustomModel{}).
		Complete(r)
}
