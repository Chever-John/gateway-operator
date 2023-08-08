package controllers

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	operatorv1alpha1 "github.com/kong/gateway-operator/apis/v1alpha1"
	operatorv1beta1 "github.com/kong/gateway-operator/apis/v1beta1"
	k8sutils "github.com/kong/gateway-operator/internal/utils/kubernetes"
)

// markAsProvisioned marks the provided resource as ready by the means of Provisioned
// Status Condition.
func markAsProvisioned[T *operatorv1beta1.DataPlane | *operatorv1alpha1.ControlPlane](resource T) {
	switch resource := any(resource).(type) {
	case *operatorv1beta1.DataPlane:
		k8sutils.SetCondition(
			k8sutils.NewConditionWithGeneration(
				DataPlaneConditionTypeProvisioned,
				metav1.ConditionTrue,
				DataPlaneConditionReasonPodsReady,
				"pods for all Deployments are ready",
				resource.Generation,
			),
			resource,
		)
	case *operatorv1alpha1.ControlPlane:
		k8sutils.SetCondition(
			k8sutils.NewConditionWithGeneration(
				ControlPlaneConditionTypeProvisioned,
				metav1.ConditionTrue,
				ControlPlaneConditionReasonPodsReady,
				"pods for all Deployments are ready",
				resource.Generation,
			),
			resource,
		)
	}
}