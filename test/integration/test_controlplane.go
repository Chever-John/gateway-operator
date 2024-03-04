package integration

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorv1beta1 "github.com/kong/gateway-operator/apis/v1beta1"
	"github.com/kong/gateway-operator/pkg/consts"
	k8sutils "github.com/kong/gateway-operator/pkg/utils/kubernetes"
	k8sresources "github.com/kong/gateway-operator/pkg/utils/kubernetes/resources"
	testutils "github.com/kong/gateway-operator/pkg/utils/test"
	"github.com/kong/gateway-operator/test/helpers"
)

func TestControlPlaneWhenNoDataPlane(t *testing.T) {
	t.Parallel()
	namespace, cleaner := helpers.SetupTestEnv(t, GetCtx(), GetEnv())

	dataplaneClient := GetClients().OperatorClient.ApisV1beta1().DataPlanes(namespace.Name)
	controlplaneClient := GetClients().OperatorClient.ApisV1beta1().ControlPlanes(namespace.Name)

	controlplaneName := types.NamespacedName{
		Namespace: namespace.Name,
		Name:      uuid.NewString(),
	}
	controlplane := &operatorv1beta1.ControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: controlplaneName.Namespace,
			Name:      controlplaneName.Name,
		},
		Spec: operatorv1beta1.ControlPlaneSpec{
			ControlPlaneOptions: operatorv1beta1.ControlPlaneOptions{
				Deployment: operatorv1beta1.ControlPlaneDeploymentOptions{
					PodTemplateSpec: &corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  consts.ControlPlaneControllerContainerName,
									Image: consts.DefaultControlPlaneImage,
								},
							},
						},
					},
				},
				DataPlane: nil,
			},
		},
	}

	// Control plane needs a dataplane to exist to properly function.
	dataplaneNN := types.NamespacedName{
		Namespace: namespace.Name,
		Name:      uuid.NewString(),
	}
	dataplane := &operatorv1beta1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: dataplaneNN.Namespace,
			Name:      dataplaneNN.Name,
		},
		Spec: operatorv1beta1.DataPlaneSpec{
			DataPlaneOptions: operatorv1beta1.DataPlaneOptions{
				Deployment: operatorv1beta1.DataPlaneDeploymentOptions{
					DeploymentOptions: operatorv1beta1.DeploymentOptions{
						PodTemplateSpec: &corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  consts.DataPlaneProxyContainerName,
										Image: consts.DefaultDataPlaneImage,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	t.Log("deploying controlplane resource without dataplane attached")
	controlplane, err := controlplaneClient.Create(GetCtx(), controlplane, metav1.CreateOptions{})
	require.NoError(t, err)
	cleaner.Add(controlplane)

	t.Log("verifying controlplane state reflects lack of dataplane")
	require.Eventually(t, testutils.ControlPlaneDetectedNoDataPlane(t, GetCtx(), controlplaneName, clients), testutils.ControlPlaneCondDeadline, testutils.ControlPlaneCondTick)

	t.Log("verifying controlplane deployment has no active replicas")
	require.Eventually(t, testutils.Not(testutils.ControlPlaneHasActiveDeployment(t, GetCtx(), controlplaneName, clients)), testutils.ControlPlaneCondDeadline, testutils.ControlPlaneCondTick)

	t.Log("deploying dataplane resource")
	dataplane, err = dataplaneClient.Create(GetCtx(), dataplane, metav1.CreateOptions{})
	require.NoError(t, err)
	cleaner.Add(dataplane)

	t.Log("verifying deployments managed by the dataplane are ready")
	require.Eventually(t, testutils.DataPlaneHasActiveDeployment(t, GetCtx(), dataplaneNN, &appsv1.Deployment{}, client.MatchingLabels{
		consts.GatewayOperatorManagedByLabel: consts.DataPlaneManagedLabelValue,
	}, clients), testutils.ControlPlaneCondDeadline, testutils.ControlPlaneCondTick)

	t.Log("verifying services managed by the dataplane")
	require.Eventually(t, testutils.DataPlaneHasService(t, GetCtx(), dataplaneNN, clients, client.MatchingLabels{
		consts.GatewayOperatorManagedByLabel: consts.DataPlaneManagedLabelValue,
		consts.DataPlaneServiceTypeLabel:     string(consts.DataPlaneIngressServiceLabelValue),
	}), testutils.ControlPlaneCondDeadline, testutils.ControlPlaneCondTick)

	t.Log("attaching dataplane to controlplane")
	controlplane, err = controlplaneClient.Get(GetCtx(), controlplane.Name, metav1.GetOptions{})
	require.NoError(t, err)
	controlplane.Spec.DataPlane = &dataplane.Name
	controlplane, err = controlplaneClient.Update(GetCtx(), controlplane, metav1.UpdateOptions{})
	require.NoError(t, err)

	t.Log("verifying controlplane is now provisioned")
	require.Eventually(t, testutils.ControlPlaneIsProvisioned(t, GetCtx(), controlplaneName, clients), testutils.ControlPlaneCondDeadline, testutils.ControlPlaneCondTick)

	t.Log("verifying controlplane deployment has active replicas")
	require.Eventually(t, testutils.ControlPlaneHasActiveDeployment(t, GetCtx(), controlplaneName, clients), testutils.ControlPlaneCondDeadline, testutils.ControlPlaneCondTick)

	t.Log("removing dataplane from controlplane")
	controlplane, err = controlplaneClient.Get(GetCtx(), controlplane.Name, metav1.GetOptions{})
	require.NoError(t, err)
	controlplane.Spec.DataPlane = nil
	_, err = controlplaneClient.Update(GetCtx(), controlplane, metav1.UpdateOptions{})
	require.NoError(t, err)

	t.Log("verifying controlplane deployment has no active replicas")
	require.Eventually(t, testutils.Not(testutils.ControlPlaneHasActiveDeployment(t, GetCtx(), controlplaneName, clients)), testutils.ControlPlaneCondDeadline, testutils.ControlPlaneCondTick)
}

func TestControlPlaneEssentials(t *testing.T) {
	t.Parallel()
	namespace, cleaner := helpers.SetupTestEnv(t, GetCtx(), GetEnv())

	dataplaneClient := GetClients().OperatorClient.ApisV1beta1().DataPlanes(namespace.Name)
	controlplaneClient := GetClients().OperatorClient.ApisV1beta1().ControlPlanes(namespace.Name)

	// Control plane needs a dataplane to exist to properly function.
	dataplaneName := types.NamespacedName{
		Namespace: namespace.Name,
		Name:      uuid.NewString(),
	}
	dataplane := &operatorv1beta1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: dataplaneName.Namespace,
			Name:      dataplaneName.Name,
		},
		Spec: operatorv1beta1.DataPlaneSpec{
			DataPlaneOptions: operatorv1beta1.DataPlaneOptions{
				Deployment: operatorv1beta1.DataPlaneDeploymentOptions{
					DeploymentOptions: operatorv1beta1.DeploymentOptions{
						PodTemplateSpec: &corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  consts.DataPlaneProxyContainerName,
										Image: consts.DefaultDataPlaneImage,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	controlplaneName := types.NamespacedName{
		Namespace: namespace.Name,
		Name:      uuid.NewString(),
	}
	controlplane := &operatorv1beta1.ControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: controlplaneName.Namespace,
			Name:      controlplaneName.Name,
		},
		Spec: operatorv1beta1.ControlPlaneSpec{
			ControlPlaneOptions: operatorv1beta1.ControlPlaneOptions{
				Deployment: operatorv1beta1.ControlPlaneDeploymentOptions{
					PodTemplateSpec: &corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"label-a": "value-a",
								"label-x": "value-x",
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Env: []corev1.EnvVar{
										{
											Name:  "TEST_ENV",
											Value: "test",
										},
									},
									Name:  consts.ControlPlaneControllerContainerName,
									Image: consts.DefaultControlPlaneImage,
								},
							},
						},
					},
				},
				DataPlane: &dataplane.Name,
			},
		},
	}

	t.Log("deploying dataplane resource")
	dataplane, err := dataplaneClient.Create(GetCtx(), dataplane, metav1.CreateOptions{})
	require.NoError(t, err)
	cleaner.Add(dataplane)

	t.Log("verifying deployments managed by the dataplane are ready")
	require.Eventually(t, testutils.DataPlaneHasActiveDeployment(t, GetCtx(), dataplaneName, &appsv1.Deployment{}, client.MatchingLabels{
		consts.GatewayOperatorManagedByLabel: consts.DataPlaneManagedLabelValue,
	}, clients), testutils.ControlPlaneCondDeadline, testutils.ControlPlaneCondTick)

	t.Log("verifying services managed by the dataplane")
	require.Eventually(t, testutils.DataPlaneHasActiveService(t, GetCtx(), dataplaneName, nil, clients, client.MatchingLabels{
		consts.GatewayOperatorManagedByLabel: consts.DataPlaneManagedLabelValue,
		consts.DataPlaneServiceTypeLabel:     string(consts.DataPlaneIngressServiceLabelValue),
	}), testutils.ControlPlaneCondDeadline, testutils.ControlPlaneCondTick)

	t.Log("deploying controlplane resource")
	controlplane, err = controlplaneClient.Create(GetCtx(), controlplane, metav1.CreateOptions{})
	require.NoError(t, err)
	cleaner.Add(controlplane)

	t.Log("verifying controlplane gets marked scheduled")
	require.Eventually(t, testutils.ControlPlaneIsScheduled(t, GetCtx(), controlplaneName, GetClients().OperatorClient), testutils.ControlPlaneCondDeadline, testutils.ControlPlaneCondTick)

	t.Log("verifying controlplane owns clusterrole and clusterrolebinding")
	require.Eventually(t, testutils.ControlPlaneHasClusterRole(t, GetCtx(), controlplane, clients), testutils.ControlPlaneCondDeadline, testutils.ControlPlaneCondTick)
	require.Eventually(t, testutils.ControlPlaneHasClusterRoleBinding(t, GetCtx(), controlplane, clients), testutils.ControlPlaneCondDeadline, testutils.ControlPlaneCondTick)

	t.Log("verifying that the controlplane gets marked as provisioned")
	require.Eventually(t, testutils.ControlPlaneIsProvisioned(t, GetCtx(), controlplaneName, clients), testutils.ControlPlaneCondDeadline, testutils.ControlPlaneCondTick)

	t.Log("verifying controlplane deployment has active replicas")
	require.Eventually(t, testutils.ControlPlaneHasActiveDeployment(t, GetCtx(), controlplaneName, clients), testutils.ControlPlaneCondDeadline, testutils.ControlPlaneCondTick)

	t.Logf("verifying that pod labels were set per the provided spec")
	require.Eventually(t, func() bool {
		deployments := testutils.MustListControlPlaneDeployments(t, GetCtx(), controlplane, clients)
		require.Len(t, deployments, 1, "There must be only one ControlPlane deployment")
		deployment := &deployments[0]

		va, oka := deployment.Spec.Template.Labels["label-a"]
		if !oka || va != "value-a" {
			t.Logf("got unexpected %q label-a value", va)
			return false
		}
		vx, okx := deployment.Spec.Template.Labels["label-x"]
		if !okx || vx != "value-x" {
			t.Logf("got unexpected %q label-x value", vx)
			return false
		}

		return true
	}, testutils.ControlPlaneCondDeadline, testutils.ControlPlaneCondTick)

	// check environment variables of deployments and pods.
	deployments := testutils.MustListControlPlaneDeployments(t, GetCtx(), controlplane, clients)
	require.Len(t, deployments, 1, "There must be only one ControlPlane deployment")
	deployment := &deployments[0]

	t.Log("verifying controlplane Deployment.Pods.Env vars")
	checkControlPlaneDeploymentEnvVars(t, deployment, controlplane.Name)

	t.Log("deleting the controlplane ClusterRole")
	clusterRoles := testutils.MustListControlPlaneClusterRoles(t, GetCtx(), controlplane, clients)
	require.Len(t, clusterRoles, 1, "There must be only one ControlPlane ClusterRole")
	require.NoError(t, GetClients().MgrClient.Delete(GetCtx(), &clusterRoles[0]))

	t.Log("verifying controlplane ClusterRole and ClusterRoleBinding have been re-created")
	require.Eventually(t, testutils.ControlPlaneHasClusterRole(t, GetCtx(), controlplane, clients), testutils.ControlPlaneCondDeadline, testutils.ControlPlaneCondTick)
	require.Eventually(t, testutils.ControlPlaneHasClusterRoleBinding(t, GetCtx(), controlplane, clients), testutils.ControlPlaneCondDeadline, testutils.ControlPlaneCondTick)
	require.Eventually(t, testutils.ControlPlaneCRBContainsCRAndSA(t, ctx, controlplane, clients), testutils.ControlPlaneCondDeadline, testutils.ControlPlaneCondTick)

	t.Log("deleting the controlplane ClusterRoleBinding")
	clusterRoleBindings := testutils.MustListControlPlaneClusterRoleBindings(t, GetCtx(), controlplane, clients)
	require.Len(t, clusterRoleBindings, 1, "There must be only one ControlPlane ClusterRoleBinding")
	require.NoError(t, GetClients().MgrClient.Delete(GetCtx(), &clusterRoleBindings[0]))

	t.Log("verifying controlplane ClusterRole and ClusterRoleBinding have been re-created")
	require.Eventually(t, testutils.ControlPlaneHasClusterRole(t, GetCtx(), controlplane, clients), testutils.ControlPlaneCondDeadline, testutils.ControlPlaneCondTick)
	require.Eventually(t, testutils.ControlPlaneHasClusterRoleBinding(t, GetCtx(), controlplane, clients), testutils.ControlPlaneCondDeadline, testutils.ControlPlaneCondTick)
	require.Eventually(t, testutils.ControlPlaneCRBContainsCRAndSA(t, ctx, controlplane, clients), testutils.ControlPlaneCondDeadline, testutils.ControlPlaneCondTick)

	t.Log("deleting the controlplane Deployment")
	require.NoError(t, GetClients().MgrClient.Delete(GetCtx(), deployment))

	t.Log("verifying deployments managed by the dataplane after deletion")
	require.Eventually(t, testutils.ControlPlaneHasActiveDeployment(t, GetCtx(), controlplaneName, clients), time.Minute, time.Second)

	t.Log("verifying controlplane Deployment.Pods.Env vars")
	checkControlPlaneDeploymentEnvVars(t, deployment, controlplane.Name)

	t.Log("verifying controlplane has a validating webhook service created")
	require.Eventually(t, testutils.ControlPlaneHasAdmissionWebhookService(t, GetCtx(), controlplane, clients), testutils.ControlPlaneCondDeadline, testutils.ControlPlaneCondTick)

	t.Log("verifying controlplane has a validating webhook certificate secret created")
	require.Eventually(t, testutils.ControlPlaneHasAdmissionWebhookCertificateSecret(t, GetCtx(), controlplane, clients), testutils.ControlPlaneCondDeadline, testutils.ControlPlaneCondTick)

	t.Log("verifying controlplane Deployment has validating webhook certificates mounted")
	verifyControlPlaneDeploymentAdmissionWebhookMount(t, deployment)

	// delete controlplane and verify that cluster wide resources removed.
	t.Log("verifying cluster wide resources removed after controlplane deleted")
	err = controlplaneClient.Delete(GetCtx(), controlplane.Name, metav1.DeleteOptions{})
	require.NoError(t, err)
	require.Eventually(t, testutils.Not(testutils.ControlPlaneHasClusterRole(t, GetCtx(), controlplane, clients)), testutils.ControlPlaneCondDeadline, testutils.ControlPlaneCondTick)
	require.Eventually(t, testutils.Not(testutils.ControlPlaneHasClusterRoleBinding(t, GetCtx(), controlplane, clients)), testutils.ControlPlaneCondDeadline, testutils.ControlPlaneCondTick)
	t.Log("verifying controlplane disappears after cluster resources are deleted")
	require.Eventually(t, func() bool {
		_, err := GetClients().OperatorClient.ApisV1beta1().ControlPlanes(controlplaneName.Namespace).Get(GetCtx(), controlplaneName.Name, metav1.GetOptions{})
		return k8serrors.IsNotFound(err)
	}, testutils.ControlPlaneCondDeadline, testutils.ControlPlaneCondTick,
		func() string {
			controlplane, err := GetClients().OperatorClient.ApisV1beta1().ControlPlanes(controlplaneName.Namespace).Get(GetCtx(), controlplaneName.Name, metav1.GetOptions{})
			if err != nil {
				return fmt.Sprintf("failed to get controlplane %s, error %v", controlplaneName.Name, err)
			}
			return fmt.Sprintf("last state of control plane: %#v", controlplane)
		},
	)
}

func checkControlPlaneDeploymentEnvVars(t *testing.T, deployment *appsv1.Deployment, controlplaneName string) {
	controllerContainer := k8sutils.GetPodContainerByName(&deployment.Spec.Template.Spec, consts.ControlPlaneControllerContainerName)
	require.NotNil(t, controllerContainer)

	envs := controllerContainer.Env
	t.Log("verifying env POD_NAME comes from metadata.name")
	podNameValueFrom := getEnvValueFromByName(envs, "POD_NAME")
	fieldRefMetadataName := &corev1.EnvVarSource{
		FieldRef: &corev1.ObjectFieldSelector{
			APIVersion: "v1",
			FieldPath:  "metadata.name",
		},
	}
	require.Truef(t, reflect.DeepEqual(fieldRefMetadataName, podNameValueFrom),
		"ValueFrom of POD_NAME should be the same as expected: expected %#v,actual %#v",
		fieldRefMetadataName, podNameValueFrom,
	)

	t.Log("verifying CONTROLLER_ELECTION_ID env has value configured in controlplane")
	electionIDEnvValue := getEnvValueByName(envs, "CONTROLLER_ELECTION_ID")
	require.Equal(t, fmt.Sprintf("%s.konghq.com", controlplaneName), electionIDEnvValue)

	t.Log("verifying custom env TEST_ENV has value configured in controlplane")
	testEnvValue := getEnvValueByName(envs, "TEST_ENV")
	require.Equal(t, "test", testEnvValue)

	t.Log("verifying that control plane has a validating webhook env var set")
	admissionWebhookListen := getEnvValueByName(envs, "CONTROLLER_ADMISSION_WEBHOOK_LISTEN")
	require.Equal(t, consts.ControlPlaneAdmissionWebhookEnvVarValue, admissionWebhookListen)
}

func verifyControlPlaneDeploymentAdmissionWebhookMount(t *testing.T, deployment *appsv1.Deployment) {
	volumes := deployment.Spec.Template.Spec.Volumes
	volumeFound := lo.ContainsBy(volumes, func(v corev1.Volume) bool {
		return v.Name == consts.ControlPlaneAdmissionWebhookVolumeName
	})
	require.Truef(t, volumeFound, "volume %s not found in deployment, actual: %s", consts.ControlPlaneAdmissionWebhookVolumeName, deployment.Spec.Template.Spec.Volumes)

	controllerContainer := k8sutils.GetPodContainerByName(&deployment.Spec.Template.Spec, consts.ControlPlaneControllerContainerName)
	require.NotNil(t, controllerContainer, "container %s not found in deployment", consts.ControlPlaneControllerContainerName)

	volumeMount, ok := lo.Find(controllerContainer.VolumeMounts, func(vm corev1.VolumeMount) bool {
		return vm.Name == consts.ControlPlaneAdmissionWebhookVolumeName
	})
	require.Truef(t, ok,
		"volume mount %s not found in container %s, actual: %v",
		consts.ControlPlaneAdmissionWebhookVolumeName,
		consts.ControlPlaneControllerContainerName,
		controllerContainer.VolumeMounts,
	)
	require.Equal(t, consts.ControlPlaneAdmissionWebhookVolumeMountPath, volumeMount.MountPath)
}

func TestControlPlaneUpdate(t *testing.T) {
	t.Parallel()
	namespace, cleaner := helpers.SetupTestEnv(t, GetCtx(), GetEnv())

	dataplaneClient := GetClients().OperatorClient.ApisV1beta1().DataPlanes(namespace.Name)
	controlplaneClient := GetClients().OperatorClient.ApisV1beta1().ControlPlanes(namespace.Name)

	dataplaneName := types.NamespacedName{
		Namespace: namespace.Name,
		Name:      uuid.NewString(),
	}
	dataplane := &operatorv1beta1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: dataplaneName.Namespace,
			Name:      dataplaneName.Name,
		},
		Spec: operatorv1beta1.DataPlaneSpec{
			DataPlaneOptions: operatorv1beta1.DataPlaneOptions{
				Deployment: operatorv1beta1.DataPlaneDeploymentOptions{
					DeploymentOptions: operatorv1beta1.DeploymentOptions{
						PodTemplateSpec: &corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  consts.DataPlaneProxyContainerName,
										Image: consts.DefaultDataPlaneImage,
										ReadinessProbe: &corev1.Probe{
											InitialDelaySeconds: 1,
											PeriodSeconds:       1,
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

	controlplaneName := types.NamespacedName{
		Namespace: namespace.Name,
		Name:      uuid.NewString(),
	}
	controlplane := &operatorv1beta1.ControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: controlplaneName.Namespace,
			Name:      controlplaneName.Name,
		},
		Spec: operatorv1beta1.ControlPlaneSpec{
			ControlPlaneOptions: operatorv1beta1.ControlPlaneOptions{
				Deployment: operatorv1beta1.ControlPlaneDeploymentOptions{
					PodTemplateSpec: &corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Env: []corev1.EnvVar{
										{
											Name: "TEST_ENV", Value: "before_update",
										},
									},
									Name:  consts.ControlPlaneControllerContainerName,
									Image: consts.DefaultControlPlaneImage,
									ReadinessProbe: &corev1.Probe{
										InitialDelaySeconds: 1,
										PeriodSeconds:       1,
									},
								},
							},
						},
					},
				},
				DataPlane: &dataplane.Name,
			},
		},
	}

	t.Log("deploying dataplane resource")
	dataplane, err := dataplaneClient.Create(GetCtx(), dataplane, metav1.CreateOptions{})
	require.NoError(t, err)
	cleaner.Add(dataplane)

	t.Log("verifying deployments managed by the dataplane are ready")
	require.Eventually(t,
		testutils.DataPlaneHasActiveDeployment(t, GetCtx(), dataplaneName, &appsv1.Deployment{}, client.MatchingLabels{
			consts.GatewayOperatorManagedByLabel: consts.DataPlaneManagedLabelValue,
		}, clients),
		testutils.ControlPlaneCondDeadline, testutils.ControlPlaneCondTick,
	)

	t.Log("deploying controlplane resource")
	controlplane, err = controlplaneClient.Create(GetCtx(), controlplane, metav1.CreateOptions{})
	require.NoError(t, err)
	cleaner.Add(controlplane)

	t.Log("verifying that the controlplane gets marked as provisioned")
	require.Eventually(t, testutils.ControlPlaneIsProvisioned(t, GetCtx(), controlplaneName, clients),
		testutils.ControlPlaneCondDeadline, testutils.ControlPlaneCondTick,
	)

	t.Log("verifying controlplane deployment has active replicas")
	require.Eventually(t, testutils.ControlPlaneHasActiveDeployment(t, GetCtx(), controlplaneName, clients),
		testutils.ControlPlaneCondDeadline, testutils.ControlPlaneCondTick,
	)

	// check environment variables of deployments and pods.
	deployments := testutils.MustListControlPlaneDeployments(t, GetCtx(), controlplane, clients)
	require.Len(t, deployments, 1, "There must be only one ControlPlane deployment")
	deployment := &deployments[0]

	t.Logf("verifying environment variable TEST_ENV in deployment before update")
	container := k8sutils.GetPodContainerByName(&deployment.Spec.Template.Spec, consts.ControlPlaneControllerContainerName)
	require.NotNil(t, container)
	testEnv := getEnvValueByName(container.Env, "TEST_ENV")
	require.Equal(t, "before_update", testEnv)

	t.Logf("updating controlplane resource")
	controlplane, err = controlplaneClient.Get(GetCtx(), controlplaneName.Name, metav1.GetOptions{})
	require.NoError(t, err)
	container = k8sutils.GetPodContainerByName(&controlplane.Spec.Deployment.PodTemplateSpec.Spec, consts.ControlPlaneControllerContainerName)
	require.NotNil(t, container)
	container.Env = []corev1.EnvVar{
		{
			Name: "TEST_ENV", Value: "after_update",
		},
	}
	_, err = controlplaneClient.Update(GetCtx(), controlplane, metav1.UpdateOptions{})
	require.NoError(t, err)

	t.Logf("verifying environment variable TEST_ENV in deployment after update")
	require.Eventually(t, func() bool {
		deployments := testutils.MustListControlPlaneDeployments(t, GetCtx(), controlplane, clients)
		require.Len(t, deployments, 1, "There must be only one ControlPlane deployment")
		deployment := &deployments[0]

		container := k8sutils.GetPodContainerByName(&deployment.Spec.Template.Spec, consts.ControlPlaneControllerContainerName)
		require.NotNil(t, container)
		testEnv := getEnvValueByName(container.Env, "TEST_ENV")
		t.Logf("Tenvironment variable TEST_ENV is now %s in deployment", testEnv)
		return testEnv == "after_update"
	},
		testutils.ControlPlaneCondDeadline, testutils.ControlPlaneCondTick,
	)

	t.Run("controlplane is not Ready when the underlying deployment changes state to not Ready", func(t *testing.T) {
		require.Eventually(t,
			testutils.ControlPlaneUpdateEventually(t, GetCtx(), controlplaneName, clients, func(cp *operatorv1beta1.ControlPlane) {
				cp.Spec.Deployment.PodTemplateSpec.Spec.Containers[0].Image = "kong/kubernetes-ingress-controller:99999.0.0"
			}),
			time.Minute, time.Second,
		)

		t.Logf("verifying that controlplane is indeed not Ready when the underlying deployment is not Ready")
		require.Eventually(t,
			testutils.ControlPlaneIsNotReady(t, GetCtx(), controlplaneName, clients),
			testutils.ControlPlaneCondDeadline, testutils.ControlPlaneCondTick,
		)
	})

	t.Run("controlplane gets Ready when the underlying deployment changes state to Ready", func(t *testing.T) {
		require.Eventually(t,
			testutils.ControlPlaneUpdateEventually(t, GetCtx(), controlplaneName, clients, func(cp *operatorv1beta1.ControlPlane) {
				cp.Spec.Deployment.PodTemplateSpec.Spec.Containers[0].Image = consts.DefaultControlPlaneImage
			}),
			time.Minute, time.Second,
		)

		require.Eventually(t,
			testutils.ControlPlaneIsReady(t, GetCtx(), controlplaneName, clients),
			testutils.ControlPlaneCondDeadline, testutils.ControlPlaneCondTick,
		)
	})

	t.Run("controlplane correctly reconciles when is updated with a ReadinessProbe using a port name", func(t *testing.T) {
		require.Eventually(t,
			testutils.ControlPlaneUpdateEventually(t, GetCtx(), controlplaneName, clients, func(cp *operatorv1beta1.ControlPlane) {
				container := k8sutils.GetPodContainerByName(&cp.Spec.Deployment.PodTemplateSpec.Spec, consts.ControlPlaneControllerContainerName)
				require.NotNil(t, container)
				container.ReadinessProbe = k8sresources.GenerateControlPlaneProbe("/readyz", intstr.FromInt(10254))
			}),
			time.Minute, time.Second,
		)

		require.Eventually(t,
			testutils.ControlPlaneIsReady(t, GetCtx(), controlplaneName, clients),
			testutils.ControlPlaneCondDeadline, testutils.ControlPlaneCondTick,
		)
	})
}
