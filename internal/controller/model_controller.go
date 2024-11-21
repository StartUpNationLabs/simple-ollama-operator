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
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

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
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// create a new Ollama Client
	ollamaUrl := model.Spec.OllamaUrl

	ollamaClient, err := ollama_client.NewClientWithResponses(ollamaUrl)
	if err != nil {
		logger.Error(err, "unable to create Ollama Client")
		return ctrl.Result{}, err
	}
	logger.Info("Ollama Client created")
	// If the Model is being deleted, delete it from the Ollama Client
	modelFinalizer := "model.finalizer.ollama.ollama.startupnation"

	modelName := unifyModelName(model.Spec.ModelName)

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
			logger.Info("Deleting Model", "Model Name", modelName, "Ollama URL", ollamaUrl)
			_, err := ollamaClient.DeleteApiDelete(ctx, &ollama_client.DeleteApiDeleteParams{
				Model: modelName,
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

	// Update status to indicate reconciliation has started
	if err := r.updateStatus(ctx, model, "ReconciliationStarted", metav1.ConditionTrue, "Reconciling", "Reconciliation process has started"); err != nil {
		return ctrl.Result{}, err
	}

	// get the model from the Ollama Client
	logger.Info("Checking if Model exists", "Model Name", modelName, "Ollama URL", ollamaUrl)
	res, err := ollamaClient.PostApiShowWithResponse(ctx, ollama_client.PostApiShowJSONRequestBody{
		Name: &modelName,
	})
	logger.Info("Checking if Model exists", "Model Name", modelName, "Ollama URL", ollamaUrl, "status", res.Status())
	if err == nil && res.StatusCode() == 200 {
		logger.Info("Model exists", "Model Name", modelName, "Ollama URL", ollamaUrl)
		if res.JSON200 != nil {
			logger.Info("Model exists", "Model Params", res.JSON200.Parameters, "Ollama URL", ollamaUrl)
			// Update status to indicate model exists
			if err := r.updateStatus(ctx, model, "ModelExists", metav1.ConditionTrue, "ModelFound", "Model exists in Ollama"); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	// if the model does not exist, create it
	logger.Info("Model does not exist, creating Model", "Model Name", modelName, "Ollama URL", ollamaUrl)
	stream := false
	_, err = ollamaClient.PostApiPull(ctx, ollama_client.PostApiPullJSONRequestBody{
		Name:   &modelName,
		Stream: &stream,
	})
	if err != nil {
		logger.Error(err, "unable to create Model")
		// Update status to indicate model creation failed
		if err := r.updateStatus(ctx, model, "ModelCreationFailed", metav1.ConditionTrue, "CreationFailed", "Failed to create model in Ollama"); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, err
	}
	logger.Info("Model created", "Model Name", modelName, "Ollama URL", ollamaUrl)
	// Update status to indicate model creation succeeded
	if err := r.updateStatus(ctx, model, "ModelCreated", metav1.ConditionTrue, "CreationSucceeded", "Model created successfully in Ollama"); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// updateStatus is a helper function to update the status conditions of the Model
func (r *ModelReconciler) updateStatus(ctx context.Context, model *ollamav1.Model, conditionType string, status metav1.ConditionStatus, reason string, message string) error {
	model.Status.Conditions = append(model.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	})
	return r.Status().Update(ctx, model)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ModelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ollamav1.Model{}).
		Complete(r)
}
