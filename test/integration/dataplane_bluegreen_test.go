//go:build integration_tests_bluegreen

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorv1beta1 "github.com/kong/gateway-operator/apis/v1beta1"
	"github.com/kong/gateway-operator/internal/consts"
	k8sutils "github.com/kong/gateway-operator/internal/utils/kubernetes"
	testutils "github.com/kong/gateway-operator/internal/utils/test"
	"github.com/kong/gateway-operator/test/helpers"
)

func TestDataPlaneBlueGreenRollout(t *testing.T) {
	const (
		waitTime = time.Minute
		tickTime = 100 * time.Millisecond
	)

	namespace, cleaner := helpers.SetupTestEnv(t, ctx, env)

	t.Log("deploying dataplane resource with 1 replica")
	dataplaneName := types.NamespacedName{
		Namespace: namespace.Name,
		Name:      uuid.NewString(),
	}
	dataplane := &operatorv1beta1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: dataplaneName.Namespace,
			Name:      dataplaneName.Name,
		},
		Spec: testBlueGreenDataPlaneSpec(),
	}

	dataplaneClient := clients.OperatorClient.ApisV1beta1().DataPlanes(namespace.Name)

	dataplane, err := dataplaneClient.Create(ctx, dataplane, metav1.CreateOptions{})
	require.NoError(t, err)
	cleaner.Add(dataplane)

	t.Log("verifying dataplane gets marked provisioned")
	require.Eventually(t, testutils.DataPlaneIsReady(t, ctx, dataplaneName, clients.OperatorClient), waitTime, tickTime)

	t.Run("before patching", func(t *testing.T) {
		t.Log("verifying preview deployment managed by the dataplane is present")
		require.Eventually(t, testutils.DataPlaneHasDeployment(t, ctx, dataplaneName, clients, dataplanePreviewDeploymentLabels()), waitTime, tickTime)

		t.Run("preview Admin API service", func(t *testing.T) {
			t.Log("verifying preview admin service managed by the dataplane is present")
			require.Eventually(t, testutils.DataPlaneHasService(t, ctx, dataplaneName, clients, dataplaneAdminPreviewServiceLabels()), waitTime, tickTime)

			t.Log("verifying that preview admin service has no active endpoints by default")
			adminServices := testutils.MustListDataPlaneServices(t, ctx, dataplane, clients.MgrClient, dataplaneAdminPreviewServiceLabels())
			require.Len(t, adminServices, 1)
			adminSvcNN := client.ObjectKeyFromObject(&adminServices[0])
			require.Eventually(t, testutils.DataPlaneServiceHasNActiveEndpoints(t, ctx, adminSvcNN, clients, 0), waitTime, tickTime,
				"with default rollout resource plan for DataPlane, the preview Admin Service shouldn't get an active endpoint")
		})

		t.Run("preview ingress service", func(t *testing.T) {
			t.Log("verifying preview ingress service managed by the dataplane is present")
			require.Eventually(t, testutils.DataPlaneHasService(t, ctx, dataplaneName, clients, dataplaneIngressPreviewServiceLabels()), waitTime, tickTime)

			t.Log("verifying that preview ingress service has no active endpoints by default")
			ingressServices := testutils.MustListDataPlaneServices(t, ctx, dataplane, clients.MgrClient, dataplaneIngressPreviewServiceLabels())
			require.Len(t, ingressServices, 1)
			ingressSvcNN := client.ObjectKeyFromObject(&ingressServices[0])
			require.Eventually(t, testutils.DataPlaneServiceHasNActiveEndpoints(t, ctx, ingressSvcNN, clients, 0), waitTime, tickTime,
				"with default rollout resource plan for DataPlane, the preview ingress Service shouldn't get an active endpoint")
		})
	})

	const dataplaneImageToPatch = "kong:3.1"

	t.Run("after patching", func(t *testing.T) {
		t.Logf("patching DataPlane with image %q", dataplaneImageToPatch)
		patchDataPlaneImage(ctx, t, dataplane, clients.MgrClient, dataplaneImageToPatch)

		t.Log("verifying preview deployment managed by the dataplane is present and has AvailableReplicas")
		require.Eventually(t, testutils.DataPlaneHasActiveDeployment(t, ctx, dataplaneName, &appsv1.Deployment{}, dataplanePreviewDeploymentLabels(), clients), waitTime, tickTime)

		t.Run("preview Admin API service", func(t *testing.T) {
			t.Log("verifying preview admin service managed by the dataplane has an active endpoint")
			require.Eventually(t, testutils.DataPlaneHasService(t, ctx, dataplaneName, clients, dataplaneAdminPreviewServiceLabels()), waitTime, tickTime)

			t.Log("verifying that preview admin service has an active endpoint")
			adminServices := testutils.MustListDataPlaneServices(t, ctx, dataplane, clients.MgrClient, dataplaneAdminPreviewServiceLabels())
			require.Len(t, adminServices, 1)
			adminSvcNN := client.ObjectKeyFromObject(&adminServices[0])
			require.Eventually(t, testutils.DataPlaneServiceHasNActiveEndpoints(t, ctx, adminSvcNN, clients, 1), waitTime, tickTime,
				"with default rollout resource plan for DataPlane, the preview Admin Service should get an active endpoint")
		})

		t.Run("preview ingress service", func(t *testing.T) {
			t.Log("verifying preview ingress service managed by the dataplane has an active endpoint")
			require.Eventually(t, testutils.DataPlaneHasService(t, ctx, dataplaneName, clients, dataplaneIngressPreviewServiceLabels()), waitTime, tickTime)

			t.Log("verifying that preview ingress service has an active endpoint")
			ingressServices := testutils.MustListDataPlaneServices(t, ctx, dataplane, clients.MgrClient, dataplaneIngressPreviewServiceLabels())
			require.Len(t, ingressServices, 1)
			ingressSvcNN := client.ObjectKeyFromObject(&ingressServices[0])
			require.Eventually(t, testutils.DataPlaneServiceHasNActiveEndpoints(t, ctx, ingressSvcNN, clients, 1), waitTime, tickTime,
				"with default rollout resource plan for DataPlane, the preview ingress Service should get an active endpoint")
		})

		t.Run("live ingress service", func(t *testing.T) {
			t.Log("verifying that live ingress service managed by the dataplane is available")
			var liveIngressService corev1.Service
			require.Eventually(t, testutils.DataPlaneHasActiveService(t, ctx, dataplaneName, &liveIngressService, clients, dataplaneIngressLiveServiceLabels()), waitTime, tickTime)

			require.Eventually(t, testutils.DataPlaneServiceHasNActiveEndpoints(t, ctx, client.ObjectKeyFromObject(&liveIngressService), clients, 1), waitTime, tickTime,
				"live ingress Service should always have an active endpoint")
		})

		t.Run("live deployment", func(t *testing.T) {
			t.Log("verifying live deployment managed by the dataplane is present and has an available replica")
			require.Eventually(t, testutils.DataPlaneHasActiveDeployment(t, ctx, dataplaneName, &appsv1.Deployment{}, dataplaneLiveDeploymentLabels(), clients), waitTime, tickTime)
		})
	})

	t.Run("checking that DataPlane rollout status has AwaitingPromotion Reason set for RolledOut condition", func(t *testing.T) {
		dataPlaneRolloutStatusConditionPredicate := func(c *metav1.Condition) func(dataplane *operatorv1beta1.DataPlane) bool {
			return func(dataplane *operatorv1beta1.DataPlane) bool {
				for _, condition := range dataplane.Status.RolloutStatus.Conditions {
					if condition.Type == c.Type && condition.Status == c.Status {
						return true
					}
					t.Logf("DataPlane Rollout Status condition: Type=%q;Reason:%q;Status:%q;Message:%q",
						condition.Type, condition.Reason, condition.Status, condition.Message,
					)
				}
				return false
			}
		}
		isAwaitingPromotion := dataPlaneRolloutStatusConditionPredicate(&metav1.Condition{
			Type:   string(consts.DataPlaneConditionTypeRolledOut),
			Reason: string(consts.DataPlaneConditionReasonRolloutAwaitingPromotion),
			Status: metav1.ConditionFalse,
		})
		require.Eventually(t,
			testutils.DataPlanePredicate(t, ctx, dataplaneName, isAwaitingPromotion, clients.OperatorClient),
			waitTime, tickTime,
		)
	})

	t.Run("after promotion", func(t *testing.T) {
		t.Logf("patching DataPlane with promotion triggering annotation %s=%s", operatorv1beta1.DataPlanePromoteWhenReadyAnnotationKey, operatorv1beta1.DataPlanePromoteWhenReadyAnnotationTrue)
		patchDataPlaneAnnotations(t, dataplane, clients.MgrClient, map[string]string{
			operatorv1beta1.DataPlanePromoteWhenReadyAnnotationKey: operatorv1beta1.DataPlanePromoteWhenReadyAnnotationTrue,
		})

		t.Run("live deployment", func(t *testing.T) {
			t.Log("verifying live deployment managed by the dataplane is present and has an available replica using the patched proxy image")

			require.Eventually(t,
				testutils.DataPlaneHasDeployment(t, ctx, dataplaneName, clients, dataplaneLiveDeploymentLabels(),
					func(d appsv1.Deployment) bool {
						proxyContainer := k8sutils.GetPodContainerByName(&d.Spec.Template.Spec, consts.DataPlaneProxyContainerName)
						return proxyContainer != nil && dataplaneImageToPatch == proxyContainer.Image
					},
				),
				waitTime, tickTime)
		})

		t.Run("live ingress service", func(t *testing.T) {
			t.Log("verifying that live ingress service managed by the dataplane still has active endpoints")
			var liveIngressService corev1.Service
			require.Eventually(t, testutils.DataPlaneHasActiveService(t, ctx, dataplaneName, &liveIngressService, clients, dataplaneIngressLiveServiceLabels()), waitTime, tickTime)
			require.NotNil(t, liveIngressService)

			require.Eventually(t, testutils.DataPlaneServiceHasNActiveEndpoints(t, ctx, client.ObjectKeyFromObject(&liveIngressService), clients, 1), waitTime, tickTime,
				"live ingress Service should always have an active endpoint")
		})

		t.Run(fmt.Sprintf("%s annotation is cleared from DataPlane", operatorv1beta1.DataPlanePromoteWhenReadyAnnotationKey), func(t *testing.T) {
			require.Eventually(t,
				testutils.DataPlanePredicate(t, ctx, dataplaneName,
					func(dataplane *operatorv1beta1.DataPlane) bool {
						_, ok := dataplane.Annotations[operatorv1beta1.DataPlanePromoteWhenReadyAnnotationKey]
						return !ok
					},
					clients.OperatorClient,
				),
				waitTime, tickTime,
			)
		})
	})
}

func TestDataPlane_ResourcesNotDeletedUntilOwnerIsRemoved(t *testing.T) {
	const (
		waitTime = time.Minute
		tickTime = 100 * time.Millisecond
	)

	namespace, cleaner := helpers.SetupTestEnv(t, ctx, env)

	t.Log("deploying dataplane")
	dataplane := &operatorv1beta1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace.Name,
			Name:      uuid.NewString(),
		},
		Spec: testBlueGreenDataPlaneSpec(),
	}
	dataplaneName := client.ObjectKeyFromObject(dataplane)

	dataplaneClient := clients.OperatorClient.ApisV1beta1().DataPlanes(namespace.Name)
	dataplane, err := dataplaneClient.Create(ctx, dataplane, metav1.CreateOptions{})
	require.NoError(t, err)
	cleaner.Add(dataplane)

	t.Log("ensuring all live dependent resources are created")
	var (
		liveIngressService = &corev1.Service{}
		liveAdminService   = &corev1.Service{}
		liveDeployment     = &appsv1.Deployment{}
		liveTLSSecret      = &corev1.Secret{}
	)
	require.Eventually(t, testutils.DataPlaneHasActiveService(t, ctx, dataplaneName, liveIngressService, clients, dataplaneIngressLiveServiceLabels()), waitTime, tickTime)
	require.NotNil(t, liveIngressService)

	require.Eventually(t, testutils.DataPlaneHasActiveService(t, ctx, dataplaneName, liveAdminService, clients, dataplaneAdminLiveServiceLabels()), waitTime, tickTime)
	require.NotNil(t, liveAdminService)

	require.Eventually(t, testutils.DataPlaneHasActiveDeployment(t, ctx, dataplaneName, liveDeployment, dataplaneLiveDeploymentLabels(), clients), waitTime, tickTime)
	require.NotNil(t, liveDeployment)

	require.Eventually(t, testutils.DataPlaneHasServiceSecret(t, ctx, dataplaneName, client.ObjectKeyFromObject(liveAdminService), liveTLSSecret, clients), waitTime, tickTime)
	require.NotNil(t, liveTLSSecret)

	t.Log("patching dataplane with another dataplane image to trigger rollout")
	const dataplaneImageToPatch = "kong:3.1"
	patchDataPlaneImage(ctx, t, dataplane, clients.MgrClient, dataplaneImageToPatch)

	t.Log("ensuring all preview dependent resources are created")
	var (
		previewIngressService = &corev1.Service{}
		previewAdminService   = &corev1.Service{}
		previewDeployment     = &appsv1.Deployment{}
		previewTLSSecret      = &corev1.Secret{}
	)

	require.Eventually(t, testutils.DataPlaneHasActiveService(t, ctx, dataplaneName, previewIngressService, clients, dataplaneIngressPreviewServiceLabels()), waitTime, tickTime)
	require.NotNil(t, previewIngressService)

	require.Eventually(t, testutils.DataPlaneHasActiveService(t, ctx, dataplaneName, previewAdminService, clients, dataplaneAdminPreviewServiceLabels()), waitTime, tickTime)
	require.NotNil(t, previewAdminService)

	require.Eventually(t, testutils.DataPlaneHasActiveDeployment(t, ctx, dataplaneName, previewDeployment, dataplanePreviewDeploymentLabels(), clients), waitTime, tickTime)
	require.NotNil(t, previewDeployment)

	require.Eventually(t, testutils.DataPlaneHasServiceSecret(t, ctx, dataplaneName, client.ObjectKeyFromObject(previewAdminService), previewTLSSecret, clients), waitTime, tickTime)
	require.NotNil(t, previewTLSSecret)

	dependentResources := []client.Object{
		liveIngressService,
		liveAdminService,
		liveDeployment,
		liveTLSSecret,
		previewIngressService,
		previewAdminService,
		previewDeployment,
		previewTLSSecret,
	}

	t.Log("ensuring dataplane owned resources after deletion are not immediately deleted")
	for _, resource := range dependentResources {
		require.NoError(t, clients.MgrClient.Delete(ctx, resource))

		require.Eventually(t, func() bool {
			err := clients.MgrClient.Get(ctx, client.ObjectKeyFromObject(resource), resource)
			if err != nil {
				t.Logf("error getting %T: %v", resource, err)
				return false
			}

			if resource.GetDeletionTimestamp().IsZero() {
				t.Logf("%T %q has no deletion timestamp", resource, resource.GetName())
				return false
			}

			return true
		}, waitTime, tickTime, "resource %T %q should not be deleted immediately after dataplane deletion", resource, resource.GetName())
	}

	t.Log("deleting dataplane and ensuring its owned resources are deleted after that")
	require.NoError(t, clients.MgrClient.Delete(ctx, dataplane))
	for _, resource := range dependentResources {
		require.Eventually(t, func() bool {
			err := clients.MgrClient.Get(ctx, client.ObjectKeyFromObject(resource), resource)
			if err != nil && k8serrors.IsNotFound(err) {
				return true
			}
			return false
		}, waitTime, tickTime, "resource %T %q should be deleted after dataplane deletion", resource, resource.GetName())
	}
}

func testBlueGreenDataPlaneSpec() operatorv1beta1.DataPlaneSpec {
	return operatorv1beta1.DataPlaneSpec{
		DataPlaneOptions: operatorv1beta1.DataPlaneOptions{
			Deployment: operatorv1beta1.DataPlaneDeploymentOptions{
				Rollout: &operatorv1beta1.Rollout{
					Strategy: operatorv1beta1.RolloutStrategy{
						BlueGreen: &operatorv1beta1.BlueGreenStrategy{
							Promotion: operatorv1beta1.Promotion{
								Strategy: operatorv1beta1.BreakBeforePromotion,
							},
						},
					},
				},
				DeploymentOptions: operatorv1beta1.DeploymentOptions{
					PodTemplateSpec: &corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  consts.DataPlaneProxyContainerName,
									Image: consts.DefaultDataPlaneImage,
									// Make the test a bit faster.
									ReadinessProbe: &corev1.Probe{
										InitialDelaySeconds: 0,
										PeriodSeconds:       1,
										FailureThreshold:    3,
										SuccessThreshold:    1,
										TimeoutSeconds:      1,
										ProbeHandler: corev1.ProbeHandler{
											HTTPGet: &corev1.HTTPGetAction{
												Path:   "/status",
												Port:   intstr.FromInt32(consts.DataPlaneMetricsPort),
												Scheme: corev1.URISchemeHTTP,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func patchDataPlaneImage(ctx context.Context, t *testing.T, dataplane *operatorv1beta1.DataPlane, cl client.Client, image string) {
	oldDataplane := dataplane.DeepCopy()
	require.Len(t, dataplane.Spec.Deployment.PodTemplateSpec.Spec.Containers, 1)
	dataplane.Spec.Deployment.PodTemplateSpec.Spec.Containers[0].Image = image
	require.NoError(t, cl.Patch(ctx, dataplane, client.MergeFrom(oldDataplane)))
}

func patchDataPlaneAnnotations(t *testing.T, dataplane *operatorv1beta1.DataPlane, cl client.Client, annotations map[string]string) {
	oldDataplane := dataplane.DeepCopy()
	require.Len(t, dataplane.Spec.Deployment.PodTemplateSpec.Spec.Containers, 1)
	if dataplane.Annotations == nil {
		dataplane.Annotations = annotations
	} else {
		for k, v := range annotations {
			dataplane.Annotations[k] = v
		}
	}
	require.NoError(t, cl.Patch(ctx, dataplane, client.MergeFrom(oldDataplane)))
}

func dataplaneAdminPreviewServiceLabels() client.MatchingLabels {
	return client.MatchingLabels{
		consts.GatewayOperatorControlledLabel: consts.DataPlaneManagedLabelValue,
		consts.DataPlaneServiceTypeLabel:      string(consts.DataPlaneAdminServiceLabelValue),
		consts.DataPlaneServiceStateLabel:     consts.DataPlaneStateLabelValuePreview,
	}
}

func dataplaneAdminLiveServiceLabels() client.MatchingLabels {
	return client.MatchingLabels{
		consts.GatewayOperatorControlledLabel: consts.DataPlaneManagedLabelValue,
		consts.DataPlaneServiceTypeLabel:      string(consts.DataPlaneAdminServiceLabelValue),
		consts.DataPlaneServiceStateLabel:     consts.DataPlaneStateLabelValueLive,
	}
}

func dataplaneIngressPreviewServiceLabels() client.MatchingLabels {
	return client.MatchingLabels{
		consts.GatewayOperatorControlledLabel: consts.DataPlaneManagedLabelValue,
		consts.DataPlaneServiceTypeLabel:      string(consts.DataPlaneIngressServiceLabelValue),
		consts.DataPlaneServiceStateLabel:     consts.DataPlaneStateLabelValuePreview,
	}
}

func dataplaneIngressLiveServiceLabels() client.MatchingLabels {
	return client.MatchingLabels{
		consts.GatewayOperatorControlledLabel: consts.DataPlaneManagedLabelValue,
		consts.DataPlaneServiceTypeLabel:      string(consts.DataPlaneIngressServiceLabelValue),
		consts.DataPlaneServiceStateLabel:     consts.DataPlaneStateLabelValueLive,
	}
}

func dataplanePreviewDeploymentLabels() client.MatchingLabels {
	return client.MatchingLabels{
		consts.GatewayOperatorControlledLabel: consts.DataPlaneManagedLabelValue,
		consts.DataPlaneDeploymentStateLabel:  consts.DataPlaneStateLabelValuePreview,
	}
}

func dataplaneLiveDeploymentLabels() client.MatchingLabels {
	return client.MatchingLabels{
		consts.GatewayOperatorControlledLabel: consts.DataPlaneManagedLabelValue,
		consts.DataPlaneDeploymentStateLabel:  consts.DataPlaneStateLabelValueLive,
	}
}