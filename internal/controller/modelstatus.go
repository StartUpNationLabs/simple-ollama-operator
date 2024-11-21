package controller

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// Status enum of the Model
type ModelStatus struct {
	conditionType string
	status        metav1.ConditionStatus
	reason        string
	message       string
}

var (
	StatusReconciliationStarted = ModelStatus{"ReconciliationStarted", metav1.ConditionUnknown, "Reconciling", "Reconciliation process has started"}
	StatusModelExists           = ModelStatus{"ModelExists", metav1.ConditionTrue, "ModelFound", "Model exists in Ollama"}
	StatusCreationSucceeded     = ModelStatus{"CreationSucceeded", metav1.ConditionTrue, "ModelCreated", "Model created successfully in Ollama"}
	StatusCreationFailed        = ModelStatus{"CreationFailed", metav1.ConditionFalse, "ModelCreationFailed", "Failed to create CustomModel in Ollama"}
)
