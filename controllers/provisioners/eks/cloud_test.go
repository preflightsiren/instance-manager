/*

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

package eks

import (
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/eks"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/keikoproj/instance-manager/controllers/provisioners"
	"github.com/onsi/gomega"
)

func TestCloudDiscoveryPositive(t *testing.T) {
	var (
		g       = gomega.NewGomegaWithT(t)
		k       = MockKubernetesClientSet()
		ig      = MockInstanceGroup()
		asgMock = NewAutoScalingMocker()
		iamMock = NewIamMocker()
		eksMock = NewEksMocker()
		ec2Mock = NewEc2Mocker()
	)

	w := MockAwsWorker(asgMock, iamMock, eksMock, ec2Mock)
	ctx := MockContext(ig, k, w)
	state := ctx.GetDiscoveredState()
	status := ig.GetStatus()
	configuration := ig.GetEKSConfiguration()

	iamMock.Role = &iam.Role{
		RoleName: aws.String("some-role"),
		Arn:      aws.String("some-arn"),
	}

	iamMock.InstanceProfile = &iam.InstanceProfile{
		InstanceProfileName: aws.String("some-profile"),
	}

	var (
		clusterName           = "some-cluster"
		resourceName          = "some-instance-group"
		resourceNamespace     = "default"
		launchConfigName      = "some-launch-configuration"
		ownedScalingGroupName = "scaling-group-1"
		vpcId                 = "vpc-1234567890"
		ownershipTag          = MockTagDescription(provisioners.TagClusterName, clusterName)
		nameTag               = MockTagDescription(provisioners.TagInstanceGroupName, resourceName)
		namespaceTag          = MockTagDescription(provisioners.TagInstanceGroupNamespace, resourceNamespace)
		ownedScalingGroup     = MockScalingGroup(ownedScalingGroupName, ownershipTag, nameTag, namespaceTag)
	)

	ig.SetName(resourceName)
	ig.SetNamespace(resourceNamespace)
	configuration.SetClusterName(clusterName)

	asgMock.AutoScalingGroups = []*autoscaling.Group{
		ownedScalingGroup,
		MockScalingGroup("scaling-group-2", ownershipTag),
		MockScalingGroup("scaling-group-3", ownershipTag),
	}

	launchConfig := &autoscaling.LaunchConfiguration{
		LaunchConfigurationName: aws.String(launchConfigName),
	}
	asgMock.LaunchConfigurations = []*autoscaling.LaunchConfiguration{
		launchConfig,
	}

	eksMock.EksCluster = &eks.Cluster{
		ResourcesVpcConfig: &eks.VpcConfigResponse{
			VpcId: aws.String(vpcId),
		},
	}

	err := ctx.CloudDiscovery()
	g.Expect(err).NotTo(gomega.HaveOccurred())

	lc := state.ScalingConfiguration.Resource().(*autoscaling.LaunchConfiguration)

	g.Expect(state.GetRole()).To(gomega.Equal(iamMock.Role))
	g.Expect(state.GetInstanceProfile()).To(gomega.Equal(iamMock.InstanceProfile))
	g.Expect(state.GetOwnedScalingGroups()).To(gomega.Equal(asgMock.AutoScalingGroups))
	g.Expect(state.IsProvisioned()).To(gomega.BeTrue())
	g.Expect(state.GetScalingGroup()).To(gomega.Equal(ownedScalingGroup))
	g.Expect(lc).To(gomega.Equal(launchConfig))
	g.Expect(state.ScalingConfiguration.Name()).To(gomega.Equal(launchConfigName))
	g.Expect(state.GetVPCId()).To(gomega.Equal(vpcId))
	g.Expect(status.GetNodesArn()).To(gomega.Equal(aws.StringValue(iamMock.Role.Arn)))
	g.Expect(status.GetActiveScalingGroupName()).To(gomega.Equal(ownedScalingGroupName))
	g.Expect(status.GetActiveLaunchConfigurationName()).To(gomega.Equal(launchConfigName))
	g.Expect(status.GetCurrentMin()).To(gomega.Equal(3))
	g.Expect(status.GetCurrentMax()).To(gomega.Equal(6))
}

func TestCloudDiscoveryExistingRole(t *testing.T) {
	var (
		g       = gomega.NewGomegaWithT(t)
		k       = MockKubernetesClientSet()
		ig      = MockInstanceGroup()
		asgMock = NewAutoScalingMocker()
		iamMock = NewIamMocker()
		eksMock = NewEksMocker()
		ec2Mock = NewEc2Mocker()
	)

	w := MockAwsWorker(asgMock, iamMock, eksMock, ec2Mock)
	ctx := MockContext(ig, k, w)
	configuration := ig.GetEKSConfiguration()
	state := ctx.GetDiscoveredState()

	iamMock.Role = &iam.Role{
		RoleName: aws.String("some-role"),
		Arn:      aws.String("some-arn"),
	}

	iamMock.InstanceProfile = &iam.InstanceProfile{
		InstanceProfileName: aws.String("some-profile"),
	}

	configuration.SetRoleName("some-role")
	configuration.SetInstanceProfileName("some-profile")

	err := ctx.CloudDiscovery()
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(state.GetRole()).To(gomega.Equal(iamMock.Role))
	g.Expect(state.GetInstanceProfile()).To(gomega.Equal(iamMock.InstanceProfile))
}

func TestCloudDiscoverySpotPrice(t *testing.T) {
	var (
		g       = gomega.NewGomegaWithT(t)
		k       = MockKubernetesClientSet()
		ig      = MockInstanceGroup()
		asgMock = NewAutoScalingMocker()
		iamMock = NewIamMocker()
		eksMock = NewEksMocker()
		ec2Mock = NewEc2Mocker()
	)

	w := MockAwsWorker(asgMock, iamMock, eksMock, ec2Mock)
	ctx := MockContext(ig, k, w)
	status := ig.GetStatus()
	configuration := ig.GetEKSConfiguration()

	iamMock.Role = &iam.Role{
		RoleName: aws.String("some-role"),
		Arn:      aws.String("some-arn"),
	}

	iamMock.InstanceProfile = &iam.InstanceProfile{
		InstanceProfileName: aws.String("some-profile"),
	}

	var (
		clusterName           = "some-cluster"
		resourceName          = "some-instance-group"
		resourceNamespace     = "default"
		ownedScalingGroupName = "scaling-group-1"
		ownershipTag          = MockTagDescription(provisioners.TagClusterName, clusterName)
		nameTag               = MockTagDescription(provisioners.TagInstanceGroupName, resourceName)
		namespaceTag          = MockTagDescription(provisioners.TagInstanceGroupNamespace, resourceNamespace)
	)

	ig.SetName(resourceName)
	ig.SetNamespace(resourceNamespace)
	configuration.SetClusterName(clusterName)
	mockAsg := []*autoscaling.Group{
		MockScalingGroup(ownedScalingGroupName, ownershipTag, nameTag, namespaceTag),
	}
	asgMock.AutoScalingGroups = mockAsg

	asgMock.LaunchConfigurations = []*autoscaling.LaunchConfiguration{
		{
			LaunchConfigurationName: aws.String("some-launch-configuration"),
		},
	}

	configuration.SetSpotPrice("0.67")

	err := ctx.CloudDiscovery()
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(status.GetLifecycle()).To(gomega.Equal("spot"))

	status.SetUsingSpotRecommendation(true)
	_, err = k.Kubernetes.CoreV1().Events("").Create(MockSpotEvent("1", ownedScalingGroupName, "0.80", true, time.Now()))
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// recommendation should not be used if nodes are not provisioned yet
	asgMock.AutoScalingGroups = []*autoscaling.Group{}
	err = ctx.CloudDiscovery()
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(configuration.GetSpotPrice()).To(gomega.Equal("0.67"))

	asgMock.AutoScalingGroups = mockAsg
	err = ctx.CloudDiscovery()
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(configuration.GetSpotPrice()).To(gomega.Equal("0.80"))

	_, err = k.Kubernetes.CoreV1().Events("").Create(MockSpotEvent("2", ownedScalingGroupName, "0.90", false, time.Now().Add(time.Minute*time.Duration(3))))
	g.Expect(err).NotTo(gomega.HaveOccurred())

	err = ctx.CloudDiscovery()
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(configuration.GetSpotPrice()).To(gomega.BeEmpty())
}

func TestLaunchConfigDeletion(t *testing.T) {
	var (
		g       = gomega.NewGomegaWithT(t)
		k       = MockKubernetesClientSet()
		ig      = MockInstanceGroup()
		asgMock = NewAutoScalingMocker()
		iamMock = NewIamMocker()
		eksMock = NewEksMocker()
		ec2Mock = NewEc2Mocker()
	)

	w := MockAwsWorker(asgMock, iamMock, eksMock, ec2Mock)
	ctx := MockContext(ig, k, w)
	configuration := ig.GetEKSConfiguration()

	iamMock.Role = &iam.Role{
		RoleName: aws.String("some-role"),
		Arn:      aws.String("some-arn"),
	}

	iamMock.InstanceProfile = &iam.InstanceProfile{
		InstanceProfileName: aws.String("some-profile"),
	}

	var (
		clusterName           = "some-cluster"
		resourceName          = "some-instance-group"
		resourceNamespace     = "default"
		ownedScalingGroupName = "scaling-group-1"
		ownershipTag          = MockTagDescription(provisioners.TagClusterName, clusterName)
		nameTag               = MockTagDescription(provisioners.TagInstanceGroupName, resourceName)
		namespaceTag          = MockTagDescription(provisioners.TagInstanceGroupNamespace, resourceNamespace)
	)

	ig.SetName(resourceName)
	ig.SetNamespace(resourceNamespace)
	configuration.SetClusterName(clusterName)

	asgMock.AutoScalingGroups = []*autoscaling.Group{
		MockScalingGroup(ownedScalingGroupName, ownershipTag, nameTag, namespaceTag),
	}

	asgMock.LaunchConfigurations = []*autoscaling.LaunchConfiguration{
		{
			LaunchConfigurationName: aws.String(fmt.Sprintf("%v-123456", ctx.ResourcePrefix)),
			CreatedTime:             aws.Time(time.Now()),
		},
		{
			LaunchConfigurationName: aws.String(fmt.Sprintf("%v-123457", ctx.ResourcePrefix)),
			CreatedTime:             aws.Time(time.Now().Add(time.Duration(-1) * time.Minute)),
		},
		{
			LaunchConfigurationName: aws.String(fmt.Sprintf("%v-123458", ctx.ResourcePrefix)),
			CreatedTime:             aws.Time(time.Now().Add(time.Duration(-3) * time.Minute)),
		},
		{
			LaunchConfigurationName: aws.String(fmt.Sprintf("%v-123459", ctx.ResourcePrefix)),
			CreatedTime:             aws.Time(time.Now().Add(time.Duration(-5) * time.Minute)),
		},
	}

	err := ctx.CloudDiscovery()
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(asgMock.DeleteLaunchConfigurationCallCount).To(gomega.Equal(2))
}
