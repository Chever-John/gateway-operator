package gateway

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	operatorv1alpha1 "github.com/kong/gateway-operator/apis/v1alpha1"
	"github.com/kong/gateway-operator/controllers/pkg/controlplane"
	"github.com/kong/gateway-operator/pkg/consts"
	operatorerrors "github.com/kong/gateway-operator/internal/errors"
	gwtypes "github.com/kong/gateway-operator/internal/types"
	k8sutils "github.com/kong/gateway-operator/pkg/utils/kubernetes"
	"github.com/kong/gateway-operator/pkg/utils/kubernetes/resources"
	"github.com/kong/gateway-operator/pkg/vars"
)

// -----------------------------------------------------------------------------
// GatewayReconciler - Watch Predicates
// -----------------------------------------------------------------------------

func (r *Reconciler) gatewayHasMatchingGatewayClass(obj client.Object) bool {
	gateway, ok := obj.(*gwtypes.Gateway)
	if !ok {
		log.FromContext(context.Background()).Error(
			operatorerrors.ErrUnexpectedObject,
			"failed to run predicate function",
			"expected", "Gateway", "found", reflect.TypeOf(obj),
		)
		return false
	}

	_, err := r.verifyGatewayClassSupport(context.Background(), gateway)
	if err != nil {
		// filtering here is just an optimization, the reconciler will check the
		// class as well. If we fail here it's most likely because of some failure
		// of the Kubernetes API and it's technically better to enqueue the object
		// than to drop it for eventual consistency during cluster outages.
		return !errors.Is(err, operatorerrors.ErrUnsupportedGateway)
	}

	return true
}

func (r *Reconciler) gatewayClassMatchesController(obj client.Object) bool {
	gatewayClass, ok := obj.(*gatewayv1.GatewayClass)
	if !ok {
		log.FromContext(context.Background()).Error(
			operatorerrors.ErrUnexpectedObject,
			"failed to run predicate function",
			"expected", "GatewayClass", "found", reflect.TypeOf(obj),
		)
		return false
	}

	return string(gatewayClass.Spec.ControllerName) == vars.ControllerName()
}

func (r *Reconciler) gatewayConfigurationMatchesController(obj client.Object) bool {
	ctx := context.Background()

	gatewayClassList := new(gatewayv1.GatewayClassList)
	if err := r.Client.List(ctx, gatewayClassList); err != nil {
		log.FromContext(ctx).Error(
			operatorerrors.ErrUnexpectedObject,
			"failed to run predicate function",
			"expected", "GatewayClass", "found", reflect.TypeOf(obj),
		)
		// filtering here is just an optimization, the reconciler will check the
		// class as well. If we fail here it's most likely because of some failure
		// of the Kubernetes API and it's technically better to enqueue the object
		// than to drop it for eventual consistency during cluster outages.
		return true
	}

	for _, gatewayClass := range gatewayClassList.Items {
		if string(gatewayClass.Spec.ControllerName) == vars.ControllerName() {
			return true
		}
	}

	return false
}

// -----------------------------------------------------------------------------
// GatewayReconciler - Watch Map Funcs
// -----------------------------------------------------------------------------

func (r *Reconciler) listGatewaysForGatewayClass(ctx context.Context, obj client.Object) (recs []reconcile.Request) {
	gatewayClass, ok := obj.(*gatewayv1.GatewayClass)
	if !ok {
		log.FromContext(ctx).Error(
			operatorerrors.ErrUnexpectedObject,
			"failed to run map funcs",
			"expected", "GatewayClass", "found", reflect.TypeOf(obj),
		)
		return
	}

	gateways := new(gatewayv1.GatewayList)
	if err := r.Client.List(ctx, gateways); err != nil {
		log.FromContext(ctx).Error(err, "could not list gateways in map func")
		return
	}

	for _, gateway := range gateways.Items {
		if gateway.Spec.GatewayClassName == gatewayv1.ObjectName(gatewayClass.Name) {
			recs = append(recs, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: gateway.Namespace,
					Name:      gateway.Name,
				},
			})
		}
	}

	return
}

func (r *Reconciler) listGatewaysForGatewayConfig(ctx context.Context, obj client.Object) (recs []reconcile.Request) {
	logger := log.FromContext(ctx)

	gatewayConfig, ok := obj.(*operatorv1alpha1.GatewayConfiguration)
	if !ok {
		logger.Error(
			operatorerrors.ErrUnexpectedObject,
			"failed to run map funcs",
			"expected", "GatewayConfiguration", "found", reflect.TypeOf(obj),
		)
		return
	}

	gatewayClassList := new(gatewayv1.GatewayClassList)
	if err := r.Client.List(ctx, gatewayClassList); err != nil {
		log.FromContext(ctx).Error(
			fmt.Errorf("unexpected error occurred while listing GatewayClass resources"),
			"failed to run map funcs",
			"error", err.Error(),
		)
		return
	}

	matchingGatewayClasses := make(map[string]struct{})
	for _, gatewayClass := range gatewayClassList.Items {
		if gatewayClass.Spec.ParametersRef != nil &&
			string(gatewayClass.Spec.ParametersRef.Group) == operatorv1alpha1.SchemeGroupVersion.Group &&
			string(gatewayClass.Spec.ParametersRef.Kind) == "GatewayConfiguration" &&
			gatewayClass.Spec.ParametersRef.Name == gatewayConfig.Name {
			matchingGatewayClasses[gatewayClass.Name] = struct{}{}
		}
	}

	gatewayList := new(gatewayv1.GatewayList)
	if err := r.Client.List(ctx, gatewayList); err != nil {
		log.FromContext(ctx).Error(
			fmt.Errorf("unexpected error occurred while listing Gateway resources"),
			"failed to run map funcs",
			"error", err.Error(),
		)
		return
	}

	for _, gateway := range gatewayList.Items {
		if _, ok := matchingGatewayClasses[string(gateway.Spec.GatewayClassName)]; ok {
			recs = append(recs, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: gateway.Namespace,
					Name:      gateway.Name,
				},
			})
		}
	}

	return
}

func (r *Reconciler) setDataPlaneGatewayConfigDefaults(gatewayConfig *operatorv1alpha1.GatewayConfiguration) {
	if gatewayConfig.Spec.DataPlaneOptions == nil {
		gatewayConfig.Spec.DataPlaneOptions = new(operatorv1alpha1.GatewayConfigDataPlaneOptions)
	}
}

func (r *Reconciler) setControlPlaneGatewayConfigDefaults(gateway *gwtypes.Gateway,
	gatewayConfig *operatorv1alpha1.GatewayConfiguration,
	dataplaneName,
	dataplaneIngressServiceName,
	dataplaneAdminServiceName,
	controlPlaneName string,
) {
	dontOverride := make(map[string]struct{})
	if gatewayConfig.Spec.ControlPlaneOptions == nil {
		gatewayConfig.Spec.ControlPlaneOptions = new(operatorv1alpha1.ControlPlaneOptions)
	}
	if gatewayConfig.Spec.ControlPlaneOptions.DataPlane == nil ||
		*gatewayConfig.Spec.ControlPlaneOptions.DataPlane == "" {
		gatewayConfig.Spec.ControlPlaneOptions.DataPlane = &dataplaneName
	}

	if gatewayConfig.Spec.ControlPlaneOptions.Deployment.PodTemplateSpec == nil {
		gatewayConfig.Spec.ControlPlaneOptions.Deployment.PodTemplateSpec = &corev1.PodTemplateSpec{}
	}

	controlPlanePodTemplateSpec := gatewayConfig.Spec.ControlPlaneOptions.Deployment.PodTemplateSpec
	container := k8sutils.GetPodContainerByName(&controlPlanePodTemplateSpec.Spec, consts.ControlPlaneControllerContainerName)
	if container == nil {
		// We currently do not require an image to be specified for ControlPlanes
		// hence we need to check if it has been provided.
		// If it wasn't then add it by appending the generated ControlPlane to
		// GatewayConfiguration spec.
		// This change will not be saved in the API server (i.e. user applied resource
		// will not be changed) - which is the desired behavior - since the caller
		// only uses the changed GatewayConfiguration to generate ControlPlane resource.
		container = lo.ToPtr[corev1.Container](resources.GenerateControlPlaneContainer(consts.DefaultControlPlaneImage))
		controlPlanePodTemplateSpec.Spec.Containers = append(controlPlanePodTemplateSpec.Spec.Containers, *container)
	}
	for _, env := range container.Env {
		dontOverride[env.Name] = struct{}{}
	}

	_ = controlplane.SetDefaults(gatewayConfig.Spec.ControlPlaneOptions, dontOverride, controlplane.DefaultsArgs{
		Namespace:                   gateway.Namespace,
		DataPlaneIngressServiceName: dataplaneIngressServiceName,
		DataPlaneAdminServiceName:   dataplaneAdminServiceName,
		ManagedByGateway:            true,
		ControlPlaneName:            controlPlaneName,
	})

	setControlPlaneOptionsDefaults(gatewayConfig.Spec.ControlPlaneOptions)
}
