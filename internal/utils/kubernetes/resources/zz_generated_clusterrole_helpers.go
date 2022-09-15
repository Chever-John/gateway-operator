// This file is generated by /hack/generators/kic-clusterrole-generator. DO NOT EDIT.

package resources

import (
	"fmt"
	"strings"

	"github.com/Masterminds/semver"
	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/kong/gateway-operator/internal/consts"
	"github.com/kong/gateway-operator/internal/utils/kubernetes/resources/clusterroles"
	"github.com/kong/gateway-operator/internal/versions"
)

// -----------------------------------------------------------------------------
// ClusterRole generator helper
// -----------------------------------------------------------------------------

// GenerateNewClusterRoleForControlPlane is a helper function that extract
// the version from the tag, and returns the ClusterRole with all the needed
// permissions.
func GenerateNewClusterRoleForControlPlane(controlplaneName string, image *string) (*rbacv1.ClusterRole, error) {
	version := consts.DefaultControlPlaneTag
	var constraint *semver.Constraints

	if image != nil && *image != "" {
		parts := strings.Split(*image, ":")
		if len(parts) != 2 || parts[1] == "latest" {
			version = versions.Latest
		} else if len(parts) == 2 {
			version = parts[1]
		}
	}

	semVersion, err := semver.NewVersion(version)
	if err != nil {
		return nil, err
	}

	constraint, err = semver.NewConstraint(">=2.1,<2.2")
	if err != nil {
		return nil, err
	}
	if constraint.Check(semVersion) {
		return clusterroles.GenerateNewClusterRoleForControlPlane_ge2_1_lt2_2(controlplaneName), nil
	}

	constraint, err = semver.NewConstraint(">=2.2,<2.3")
	if err != nil {
		return nil, err
	}
	if constraint.Check(semVersion) {
		return clusterroles.GenerateNewClusterRoleForControlPlane_ge2_2_lt2_3(controlplaneName), nil
	}

	constraint, err = semver.NewConstraint(">=2.3,<2.4")
	if err != nil {
		return nil, err
	}
	if constraint.Check(semVersion) {
		return clusterroles.GenerateNewClusterRoleForControlPlane_ge2_3_lt2_4(controlplaneName), nil
	}

	constraint, err = semver.NewConstraint(">=2.4,<2.6")
	if err != nil {
		return nil, err
	}
	if constraint.Check(semVersion) {
		return clusterroles.GenerateNewClusterRoleForControlPlane_ge2_4_lt2_6(controlplaneName), nil
	}

	constraint, err = semver.NewConstraint(">=2.6")
	if err != nil {
		return nil, err
	}
	if constraint.Check(semVersion) {
		return clusterroles.GenerateNewClusterRoleForControlPlane_ge2_6(controlplaneName), nil
	}

	return nil, fmt.Errorf("version %s not supported", version)
}
