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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	// Fetch the CustomModel instance
	customModel := &ollamav1.CustomModel{}
	err := r.Get(ctx, req.NamespacedName, customModel)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// create a new Ollama Client
	ollamaUrl := customModel.Spec.OllamaUrl
	ollamaClient, err := ollama_client.NewClientWithResponses(ollamaUrl)
	if err != nil {
		logger.Error(err, "unable to create Ollama Client")
		return ctrl.Result{}, err
	}
	logger.Info("Ollama Client created")
	// If the CustomModel is being deleted, delete it from the Ollama Client
	customModelFinalizer := "custommodel.finalizer.ollama.ollama.startupnation"

	modelName := unifyModelName(customModel.Spec.ModelName)
	if customModel.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object.
		if !containsString(customModel.ObjectMeta.Finalizers, customModelFinalizer) {
			customModel.ObjectMeta.Finalizers = append(customModel.ObjectMeta.Finalizers, customModelFinalizer)
			if err := r.Update(context.Background(), customModel); err != nil {
				return reconcile.Result{}, err
			}
		}
	} else {
		// The object is being deleted
		if containsString(customModel.ObjectMeta.Finalizers, customModelFinalizer) {
			// our finalizer is present, so lets handle our external dependency
			// first, we delete the external dependency
			logger.Info("Deleting CustomModel", "CustomModel Name", modelName, "Ollama URL", ollamaUrl)
			_, err := ollamaClient.DeleteApiDelete(ctx, &ollama_client.DeleteApiDeleteParams{
				Model: modelName,
			})
			if err != nil {
				logger.Error(err, "unable to delete CustomModel")
			}

			// remove our finalizer from the list and update it.
			customModel.ObjectMeta.Finalizers = removeString(customModel.ObjectMeta.Finalizers, customModelFinalizer)
			if err := r.Update(context.Background(), customModel); err != nil {
				return reconcile.Result{}, err
			}
		}

		// Our finalizer has finished, so the reconciler can do nothing.
		return reconcile.Result{}, nil
	}
	// if the CustomModel is not being deleted, start reconciliation

	// Update status to indicate reconciliation has started
	if err := r.updateStatus(ctx, customModel, "ReconciliationStarted", metav1.ConditionTrue, "Reconciling", "Reconciliation process has started"); err != nil {
		return ctrl.Result{}, err
	}

	// get the CustomModel from the Ollama Client
	logger.Info("Checking if CustomModel exists", "CustomModel Name", modelName, "Ollama URL", ollamaUrl)
	res, err := ollamaClient.PostApiShowWithResponse(ctx, ollama_client.PostApiShowJSONRequestBody{
		Name: &modelName,
	})
	logger.Info("Checking if CustomModel exists", "CustomModel Name", modelName, "Ollama URL", ollamaUrl, "status", res.Status())
	if err == nil && res.StatusCode() == 200 {
		logger.Info("CustomModel exists", "CustomModel Name", modelName, "Ollama URL", ollamaUrl)
		if res.JSON200 != nil {
			logger.Info("Checking if the ModelFile is the same", "CustomModel Name", modelName, "Ollama URL", ollamaUrl)
			if *res.JSON200.Parameters == customModel.Spec.ModelFile {
				logger.Info("ModelFile is the same", "CustomModel Name", modelName, "Ollama URL", ollamaUrl)
				// Update status to indicate model exists
				if err := r.updateStatus(ctx, customModel, "CustomModelExists", metav1.ConditionTrue, "ModelFound", "CustomModel exists in Ollama"); err != nil {
					return ctrl.Result{}, err
				}
				return ctrl.Result{}, nil
			}
			logger.Info("ModelFile is not the same", "CustomModel Name", modelName, "Ollama URL", ollamaUrl)
			// delete the CustomModel and create a new one
			logger.Info("Deleting CustomModel", "CustomModel Name", modelName, "Ollama URL", ollamaUrl)
			_, err := ollamaClient.DeleteApiDelete(ctx, &ollama_client.DeleteApiDeleteParams{
				Model: modelName,
			})
			if err != nil {
				logger.Error(err, "unable to delete CustomModel")
			}
			// Create the CustomModel
			logger.Info("Creating CustomModel", "CustomModel Name", modelName, "Ollama URL", ollamaUrl)
			stream := false
			_, err = ollamaClient.PostApiCreate(ctx, ollama_client.PostApiCreateJSONRequestBody{
				Name:      &modelName,
				Modelfile: &customModel.Spec.ModelFile,
				Stream:    &stream,
			})
			if err != nil {
				logger.Error(err, "unable to create CustomModel")
				// Update status to indicate model creation failed
				if err := r.updateStatus(ctx, customModel, "CustomModelCreationFailed", metav1.ConditionTrue, "CreationFailed", "Failed to create CustomModel in Ollama"); err != nil {
					return ctrl.Result{}, err
				}
				return ctrl.Result{}, err
			}
		}
		// Update status to indicate model creation succeeded
		if err := r.updateStatus(ctx, customModel, "CustomModelCreated", metav1.ConditionTrue, "CreationSucceeded", "CustomModel created successfully in Ollama"); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, err
	}
	// if the CustomModel does not exist, create it
	logger.Info("CustomModel does not exist, creating CustomModel", "CustomModel Name", modelName, "Ollama URL", ollamaUrl, "ModelFile", customModel.Spec.ModelFile)
	stream := false
	_, err = ollamaClient.PostApiCreate(ctx, ollama_client.PostApiCreateJSONRequestBody{
		Name:      &modelName,
		Modelfile: &customModel.Spec.ModelFile,
		Stream:    &stream,
	})
	if err != nil || res.StatusCode() != 200 {
		logger.Error(err, "unable to create CustomModel")
		// Update status to indicate model creation failed
		if err := r.updateStatus(ctx, customModel, "CustomModelCreationFailed", metav1.ConditionTrue, "CreationFailed", "Failed to create CustomModel in Ollama"); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, err
	}
	logger.Info("CustomModel created", "CustomModel Name", modelName, "Ollama URL", ollamaUrl)
	// Update status to indicate model creation succeeded
	if err := r.updateStatus(ctx, customModel, "CustomModelCreated", metav1.ConditionTrue, "CreationSucceeded", "CustomModel created successfully in Ollama"); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// updateStatus is a helper function to update the status conditions of the CustomModel
func (r *CustomModelReconciler) updateStatus(ctx context.Context, customModel *ollamav1.CustomModel, conditionType string, status metav1.ConditionStatus, reason string, message string) error {
	customModel.Status.Conditions = append(customModel.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	})
	return r.Status().Update(ctx, customModel)
}

// SetupWithManager sets up the controller with the Manager.
func (r *CustomModelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ollamav1.CustomModel{}).
		Complete(r)
}
