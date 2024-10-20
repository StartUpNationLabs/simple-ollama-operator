package controller

import "strings"

// Helper functions to check and remove string from a slice of strings.
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func removeString(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return
}

func unifyModelName(modelName string) string {
	// check if : is present in the modelName
	if strings.Contains(modelName, ":") {
		return modelName
	}
	return modelName + ":latest"
}
