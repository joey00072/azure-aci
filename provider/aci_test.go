/**
* Copyright (c) Microsoft.  All rights reserved.
	*/

package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	azure "github.com/virtual-kubelet/azure-aci/client"
	"github.com/virtual-kubelet/azure-aci/client/aci"
	"github.com/virtual-kubelet/node-cli/manager"
	"gotest.tools/assert"

	is "gotest.tools/assert/cmp"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
)

const (
	fakeSubscription  = "a88d9e8f-3cb3-456f-8f10-27395c1e122a"
	fakeResourceGroup = "vk-rg"
	fakeClientID      = "f14193ad-4c4c-4876-a18a-c0badb3bbd40"
	fakeClientSecret  = "VGhpcyBpcyBhIHNlY3JldAo="
	fakeTenantID      = "8cb81aca-83fe-4c6f-b667-4ec09c45a8bf"
	fakeNodeName      = "vk"
	fakeRegion        = "eastus"
	fakeUserIdentity  = "00000000-0000-0000-0000-000000000000"
)

// Test make registry credential
func TestMakeRegistryCredential(t *testing.T) {
	server := "server-" + uuid.New().String()
	username := "user-" + uuid.New().String()
	password := "pass-" + uuid.New().String()
	auth := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", username, password)))

	tt := []struct {
		name        string
		authConfig  AuthConfig
		shouldFail  bool
		failMessage string
	}{
		{
			"Valid username and password",
			AuthConfig{Username: username, Password: password},
			false,
			"",
		},
		{
			"Username and password in auth",
			AuthConfig{Auth: auth},
			false,
			"",
		},
		{
			"No Username",
			AuthConfig{},
			true,
			"no username present in auth config for server",
		},
		{
			"Invalid Auth",
			AuthConfig{Auth: "123"},
			true,
			"error decoding the auth for server",
		},
		{
			"Malformed Auth",
			AuthConfig{Auth: base64.StdEncoding.EncodeToString([]byte("123"))},
			true,
			"malformed auth for server",
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			cred, err := makeRegistryCredential(server, tc.authConfig)

			if tc.shouldFail {
				assert.Check(t, err != nil, "conversion should fail")
				assert.Check(t, strings.Contains(err.Error(), tc.failMessage), "failed message is not expected")
				return
			}

			assert.Check(t, err, "conversion should not fail")
			assert.Check(t, cred != nil, "credential should not be nil")
			assert.Check(t, is.Equal(server, cred.Server), "server doesn't match")
			assert.Check(t, is.Equal(username, cred.Username), "username doesn't match")
			assert.Check(t, is.Equal(password, cred.Password), "password doesn't match")
		})
	}
}

// Tests create pod without resource spec
func TestCreatePodWithoutResourceSpec(t *testing.T) {
	_, aciServerMocker, provider, err := prepareMocks()

	if err != nil {
		t.Fatal("Unable to prepare the mocks", err)
	}

	podName := "pod-" + uuid.New().String()
	podNamespace := "ns-" + uuid.New().String()

	aciServerMocker.OnCreate = func(subscription, resourceGroup, containerGroup string, cg *aci.ContainerGroup) (int, interface{}) {
		assert.Check(t, is.Equal(fakeSubscription, subscription), "Subscription doesn't match")
		assert.Check(t, is.Equal(fakeResourceGroup, resourceGroup), "Resource group doesn't match")
		assert.Check(t, cg != nil, "Container group is nil")
		assert.Check(t, is.Equal(podNamespace+"-"+podName, containerGroup), "Container group name is not expected")
		assert.Check(t, cg.ContainerGroupProperties.Containers != nil, "Containers should not be nil")
		assert.Check(t, is.Equal(1, len(cg.ContainerGroupProperties.Containers)), "1 Container is expected")
		assert.Check(t, is.Equal("nginx", cg.ContainerGroupProperties.Containers[0].Name), "Container nginx is expected")
		assert.Check(t, cg.ContainerGroupProperties.Containers[0].Resources.Requests != nil, "Container resource requests should not be nil")
		assert.Check(t, is.Equal(1.0, cg.ContainerGroupProperties.Containers[0].Resources.Requests.CPU), "Request CPU is not expected")
		assert.Check(t, is.Equal(1.5, cg.ContainerGroupProperties.Containers[0].Resources.Requests.MemoryInGB), "Request Memory is not expected")
		assert.Check(t, is.Nil(cg.ContainerGroupProperties.Containers[0].Resources.Limits), "Limits should be nil")

		return http.StatusOK, cg
	}

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: podNamespace,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				v1.Container{
					Name: "nginx",
				},
			},
		},
	}

	if err := provider.CreatePod(context.Background(), pod); err != nil {
		t.Fatal("Failed to create pod", err)
	}
}

// Tests create pod with resource request only
func TestCreatePodWithResourceRequestOnly(t *testing.T) {
	_, aciServerMocker, provider, err := prepareMocks()

	if err != nil {
		t.Fatal("Unable to prepare the mocks", err)
	}

	podName := "pod-" + uuid.New().String()
	podNamespace := "ns-" + uuid.New().String()

	aciServerMocker.OnCreate = func(subscription, resourceGroup, containerGroup string, cg *aci.ContainerGroup) (int, interface{}) {
		assert.Check(t, is.Equal(fakeSubscription, subscription), "Subscription doesn't match")
		assert.Check(t, is.Equal(fakeResourceGroup, resourceGroup), "Resource group doesn't match")
		assert.Check(t, cg != nil, "Container group is nil")
		assert.Check(t, is.Equal(podNamespace+"-"+podName, containerGroup), "Container group name is not expected")
		assert.Check(t, cg.ContainerGroupProperties.Containers != nil, "Containers should not be nil")
		assert.Check(t, is.Equal(1, len(cg.ContainerGroupProperties.Containers)), "1 Container is expected")
		assert.Check(t, is.Equal("nginx", cg.ContainerGroupProperties.Containers[0].Name), "Container nginx is expected")
		assert.Check(t, cg.ContainerGroupProperties.Containers[0].Resources.Requests != nil, "Container resource requests should not be nil")
		assert.Check(t, is.Equal(1.98, cg.ContainerGroupProperties.Containers[0].Resources.Requests.CPU), "Request CPU is not expected")
		assert.Check(t, is.Equal(3.4, cg.ContainerGroupProperties.Containers[0].Resources.Requests.MemoryInGB), "Request Memory is not expected")
		assert.Check(t, is.Nil(cg.ContainerGroupProperties.Containers[0].Resources.Limits), "Limits should be nil")

		return http.StatusOK, cg
	}

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: podNamespace,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				v1.Container{
					Name: "nginx",
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							"cpu":    resource.MustParse("1.981"),
							"memory": resource.MustParse("3.49G"),
						},
					},
				},
			},
		},
	}

	if err := provider.CreatePod(context.Background(), pod); err != nil {
		t.Fatal("Failed to create pod", err)
	}
}

// Tests create pod with default GPU SKU.
func TestCreatePodWithGPU(t *testing.T) {
	aadServerMocker := NewAADMock()
	aciServerMocker := NewACIMock()

	podName := "pod-" + uuid.New().String()
	podNamespace := "ns-" + uuid.New().String()
	gpuSKU := aci.GPUSKU("sku-" + uuid.New().String())

	aciServerMocker.OnGetRPManifest = func() (int, interface{}) {
		manifest := &aci.ResourceProviderManifest{
			Metadata: &aci.ResourceProviderMetadata{
				GPURegionalSKUs: []*aci.GPURegionalSKU{
					&aci.GPURegionalSKU{
						Location: fakeRegion,
						SKUs:     []aci.GPUSKU{gpuSKU, aci.K80, aci.P100},
					},
				},
			},
		}

		return http.StatusOK, manifest
	}

	provider, err := createTestProvider(aadServerMocker, aciServerMocker, nil)
	if err != nil {
		t.Fatalf("failed to create the test provider. %s", err.Error())
		return
	}

	aciServerMocker.OnCreate = func(subscription, resourceGroup, containerGroup string, cg *aci.ContainerGroup) (int, interface{}) {
		assert.Check(t, is.Equal(fakeSubscription, subscription), "Subscription doesn't match")
		assert.Check(t, is.Equal(fakeResourceGroup, resourceGroup), "Resource group doesn't match")
		assert.Check(t, is.Equal(podNamespace+"-"+podName, containerGroup), "Container group name is not expected")
		assert.Check(t, cg.ContainerGroupProperties.Containers != nil, "Containers should not be nil")
		assert.Check(t, is.Equal(1, len(cg.ContainerGroupProperties.Containers)), "1 Container is expected")
		assert.Check(t, is.Equal("nginx", cg.ContainerGroupProperties.Containers[0].Name), "Container nginx is expected")
		assert.Check(t, cg.ContainerGroupProperties.Containers[0].Resources.Requests != nil, "Container resource requests should not be nil")
		assert.Check(t, is.Equal(1.98, cg.ContainerGroupProperties.Containers[0].Resources.Requests.CPU), "Request CPU is not expected")
		assert.Check(t, is.Equal(3.4, cg.ContainerGroupProperties.Containers[0].Resources.Requests.MemoryInGB), "Request Memory is not expected")
		assert.Check(t, cg.ContainerGroupProperties.Containers[0].Resources.Requests.GPU != nil, "Requests GPU is not expected")
		assert.Check(t, is.Equal(int32(10), cg.ContainerGroupProperties.Containers[0].Resources.Requests.GPU.Count), "Requests GPU Count is not expected")
		assert.Check(t, is.Equal(gpuSKU, cg.ContainerGroupProperties.Containers[0].Resources.Requests.GPU.SKU), "Requests GPU SKU is not expected")
		assert.Check(t, cg.ContainerGroupProperties.Containers[0].Resources.Limits.GPU != nil, "Limits GPU is not expected")
		assert.Check(t, is.Equal(int32(10), cg.ContainerGroupProperties.Containers[0].Resources.Limits.GPU.Count), "Requests GPU Count is not expected")
		assert.Check(t, is.Equal(gpuSKU, cg.ContainerGroupProperties.Containers[0].Resources.Limits.GPU.SKU), "Requests GPU SKU is not expected")

		return http.StatusOK, cg
	}

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: podNamespace,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				v1.Container{
					Name: "nginx",
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							"cpu":    resource.MustParse("1.981"),
							"memory": resource.MustParse("3.49G"),
						},
						Limits: v1.ResourceList{
							gpuResourceName: resource.MustParse("10"),
						},
					},
				},
			},
		},
	}

	if err := provider.CreatePod(context.Background(), pod); err != nil {
		t.Fatal("Failed to create pod", err)
	}
}

// Tests create pod with GPU SKU in annotation.
func TestCreatePodWithGPUSKU(t *testing.T) {
	aadServerMocker := NewAADMock()
	aciServerMocker := NewACIMock()

	podName := "pod-" + uuid.New().String()
	podNamespace := "ns-" + uuid.New().String()
	gpuSKU := aci.GPUSKU("sku-" + uuid.New().String())

	aciServerMocker.OnGetRPManifest = func() (int, interface{}) {
		manifest := &aci.ResourceProviderManifest{
			Metadata: &aci.ResourceProviderMetadata{
				GPURegionalSKUs: []*aci.GPURegionalSKU{
					&aci.GPURegionalSKU{
						Location: fakeRegion,
						SKUs:     []aci.GPUSKU{aci.K80, aci.P100, gpuSKU},
					},
				},
			},
		}

		return http.StatusOK, manifest
	}

	provider, err := createTestProvider(aadServerMocker, aciServerMocker, nil)
	if err != nil {
		t.Fatalf("failed to create the test provider. %s", err.Error())
		return
	}

	aciServerMocker.OnCreate = func(subscription, resourceGroup, containerGroup string, cg *aci.ContainerGroup) (int, interface{}) {
		assert.Check(t, is.Equal(fakeSubscription, subscription), "Subscription doesn't match")
		assert.Check(t, is.Equal(fakeResourceGroup, resourceGroup), "Resource group doesn't match")
		assert.Check(t, cg != nil, "Container group is nil")
		assert.Check(t, is.Equal(podNamespace+"-"+podName, containerGroup), "Container group name is not expected")
		assert.Check(t, cg.ContainerGroupProperties.Containers != nil, "Containers should not be nil")
		assert.Check(t, is.Equal(1, len(cg.ContainerGroupProperties.Containers)), "1 Container is expected")
		assert.Check(t, is.Equal("nginx", cg.ContainerGroupProperties.Containers[0].Name), "Container nginx is expected")
		assert.Check(t, cg.ContainerGroupProperties.Containers[0].Resources.Requests != nil, "Container resource requests should not be nil")
		assert.Check(t, is.Equal(1.98, cg.ContainerGroupProperties.Containers[0].Resources.Requests.CPU), "Request CPU is not expected")
		assert.Check(t, is.Equal(3.4, cg.ContainerGroupProperties.Containers[0].Resources.Requests.MemoryInGB), "Request Memory is not expected")
		assert.Check(t, cg.ContainerGroupProperties.Containers[0].Resources.Requests.GPU != nil, "Requests GPU is not expected")
		assert.Check(t, is.Equal(int32(1), cg.ContainerGroupProperties.Containers[0].Resources.Requests.GPU.Count), "Requests GPU Count is not expected")
		assert.Check(t, is.Equal(gpuSKU, cg.ContainerGroupProperties.Containers[0].Resources.Requests.GPU.SKU), "Requests GPU SKU is not expected")
		assert.Check(t, cg.ContainerGroupProperties.Containers[0].Resources.Limits.GPU != nil, "Limits GPU is not expected")
		assert.Check(t, is.Equal(int32(1), cg.ContainerGroupProperties.Containers[0].Resources.Limits.GPU.Count), "Requests GPU Count is not expected")
		assert.Check(t, is.Equal(gpuSKU, cg.ContainerGroupProperties.Containers[0].Resources.Limits.GPU.SKU), "Requests GPU SKU is not expected")

		return http.StatusOK, cg
	}

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: podNamespace,
			Annotations: map[string]string{
				gpuTypeAnnotation: string(gpuSKU),
			},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				v1.Container{
					Name: "nginx",
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							"cpu":    resource.MustParse("1.981"),
							"memory": resource.MustParse("3.49G"),
						},
						Limits: v1.ResourceList{
							gpuResourceName: resource.MustParse("1"),
						},
					},
				},
			},
		},
	}

	if err := provider.CreatePod(context.Background(), pod); err != nil {
		t.Fatal("Failed to create pod", err)
	}
}

// Tests create pod with both resource request and limit.
func TestCreatePodWithResourceRequestAndLimit(t *testing.T) {
	_, aciServerMocker, provider, err := prepareMocks()

	if err != nil {
		t.Fatal("Unable to prepare the mocks", err)
	}

	podName := "pod-" + uuid.New().String()
	podNamespace := "ns-" + uuid.New().String()

	aciServerMocker.OnCreate = func(subscription, resourceGroup, containerGroup string, cg *aci.ContainerGroup) (int, interface{}) {
		assert.Check(t, is.Equal(fakeSubscription, subscription), "Subscription doesn't match")
		assert.Check(t, is.Equal(fakeResourceGroup, resourceGroup), "Resource group doesn't match")
		assert.Check(t, cg != nil, "Container group is nil")
		assert.Check(t, is.Equal(podNamespace+"-"+podName, containerGroup), "Container group name is not expected")
		assert.Check(t, cg.ContainerGroupProperties.Containers != nil, "Containers should not be nil")
		assert.Check(t, is.Equal(1, len(cg.ContainerGroupProperties.Containers)), "1 Container is expected")
		assert.Check(t, is.Equal("nginx", cg.ContainerGroupProperties.Containers[0].Name), "Container nginx is expected")
		assert.Check(t, cg.ContainerGroupProperties.Containers[0].Resources.Requests != nil, "Container resource requests should not be nil")
		assert.Check(t, is.Equal(1.98, cg.ContainerGroupProperties.Containers[0].Resources.Requests.CPU), "Request CPU is not expected")
		assert.Check(t, is.Equal(3.4, cg.ContainerGroupProperties.Containers[0].Resources.Requests.MemoryInGB), "Request Memory is not expected")
		assert.Check(t, is.Equal(3.999, cg.ContainerGroupProperties.Containers[0].Resources.Limits.CPU), "Limit CPU is not expected")
		assert.Check(t, is.Equal(8.0, cg.ContainerGroupProperties.Containers[0].Resources.Limits.MemoryInGB), "Limit Memory is not expected")

		return http.StatusOK, cg
	}

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: podNamespace,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				v1.Container{
					Name: "nginx",
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							"cpu":    resource.MustParse("1.981"),
							"memory": resource.MustParse("3.49G"),
						},
						Limits: v1.ResourceList{
							"cpu":    resource.MustParse("3999m"),
							"memory": resource.MustParse("8010M"),
						},
					},
				},
			},
		},
	}

	if err := provider.CreatePod(context.Background(), pod); err != nil {
		t.Fatal("Failed to create pod", err)
	}
}

// Tests get pods with empty list.
func TestGetPodsWithEmptyList(t *testing.T) {
	_, aciServerMocker, provider, err := prepareMocks()

	if err != nil {
		t.Fatal("Unable to prepare the mocks", err)
	}

	aciServerMocker.OnGetContainerGroups = func(subscription, resourceGroup string) (int, interface{}) {
		assert.Check(t, is.Equal(fakeSubscription, subscription), "Subscription doesn't match")
		assert.Check(t, is.Equal(fakeResourceGroup, resourceGroup), "Resource group doesn't match")

		return http.StatusOK, aci.ContainerGroupListResult{
			Value: []aci.ContainerGroup{},
		}
	}

	pods, err := provider.GetPods(context.Background())
	if err != nil {
		t.Fatal("Failed to get pods", err)
	}

	assert.Check(t, pods != nil, "Response pods should not be nil")
	assert.Check(t, is.Equal(0, len(pods)), "No pod should be returned")
}

// Tests get pods without requests limit.
func TestGetPodsWithoutResourceRequestsLimits(t *testing.T) {
	_, aciServerMocker, provider, err := prepareMocks()

	if err != nil {
		t.Fatal("Unable to prepare the mocks", err)
	}

	aciServerMocker.OnGetContainerGroups = func(subscription, resourceGroup string) (int, interface{}) {
		assert.Check(t, is.Equal(fakeSubscription, subscription), "Subscription doesn't match")
		assert.Check(t, is.Equal(fakeResourceGroup, resourceGroup), "Resource group doesn't match")

		var cg = aci.ContainerGroup{
			Name: "default-nginx",
			Tags: map[string]string{
				"NodeName": fakeNodeName,
			},
			ContainerGroupProperties: aci.ContainerGroupProperties{
				ProvisioningState: "Creating",
				Containers: []aci.Container{
					aci.Container{
						Name: "nginx",
						ContainerProperties: aci.ContainerProperties{
							Image:   "nginx",
							Command: []string{"nginx", "-g", "daemon off;"},
							Ports: []aci.ContainerPort{
								{
									Protocol: aci.ContainerNetworkProtocolTCP,
									Port:     80,
								},
							},
							Resources: aci.ResourceRequirements{
								Requests: &aci.ComputeResources{
									CPU:        0.99,
									MemoryInGB: 1.5,
								},
							},
						},
					},
				},
			},
		}

		return http.StatusOK, aci.ContainerGroupListResult{
			Value: []aci.ContainerGroup{cg},
		}
	}

	pods, err := provider.GetPods(context.Background())
	if err != nil {
		t.Fatal("Failed to get pods", err)
	}

	assert.Check(t, pods != nil, "Response pods should not be nil")
	assert.Check(t, is.Equal(1, len(pods)), "No pod should be returned")

	pod := pods[0]

	assert.Check(t, pod != nil, "Response pod should not be nil")
	assert.Check(t, pod.Spec.Containers != nil, "Containers should not be nil")
	assert.Check(t, is.Nil(pod.Spec.Containers[0].Resources.Limits), "Containers[0].Resources.Limits should be nil")
	assert.Check(t, pod.Spec.Containers[0].Resources.Requests != nil, "Containers[0].Resources.Requests should be nil")
	assert.Check(t, is.Equal(ptrQuantity(resource.MustParse("0.99")).Value(),
		pod.Spec.Containers[0].Resources.Requests.Cpu().Value()), "Containers[0].Resources.Requests.CPU doesn't match")
	assert.Check(t, is.Equal(ptrQuantity(resource.MustParse("1.5G")).Value(),
		pod.Spec.Containers[0].Resources.Requests.Memory().Value()), "Containers[0].Resources.Requests.Memory doesn't match")
}

// Tests get pod without requests limit.
func TestGetPodWithoutResourceRequestsLimits(t *testing.T) {
	_, aciServerMocker, provider, err := prepareMocks()

	if err != nil {
		t.Fatal("Unable to prepare the mocks", err)
	}

	podName := "pod-" + uuid.New().String()
	podNamespace := "ns-" + uuid.New().String()

	aciServerMocker.OnGetContainerGroup = func(subscription, resourceGroup, containerGroup string) (int, interface{}) {
		assert.Check(t, is.Equal(fakeSubscription, subscription), "Subscription doesn't match")
		assert.Check(t, is.Equal(fakeResourceGroup, resourceGroup), "Resource group doesn't match")
		assert.Check(t, is.Equal(podNamespace+"-"+podName, containerGroup), "Container group name is not expected")

		return http.StatusOK, aci.ContainerGroup{
			Tags: map[string]string{
				"NodeName": fakeNodeName,
			},
			ContainerGroupProperties: aci.ContainerGroupProperties{
				ProvisioningState: "Creating",
				Containers: []aci.Container{
					aci.Container{
						Name: "nginx",
						ContainerProperties: aci.ContainerProperties{
							Image:   "nginx",
							Command: []string{"nginx", "-g", "daemon off;"},
							Ports: []aci.ContainerPort{
								{
									Protocol: aci.ContainerNetworkProtocolTCP,
									Port:     80,
								},
							},
							Resources: aci.ResourceRequirements{
								Requests: &aci.ComputeResources{
									CPU:        0.99,
									MemoryInGB: 1.5,
								},
							},
						},
					},
				},
			},
		}
	}

	pod, err := provider.GetPod(context.Background(), podNamespace, podName)
	if err != nil {
		t.Fatal("Failed to get pod", err)
	}

	assert.Check(t, pod != nil, "Response pod should not be nil")
	assert.Check(t, pod.Spec.Containers != nil, "Containers should not be nil")
	assert.Check(t, is.Nil(pod.Spec.Containers[0].Resources.Limits), "Containers[0].Resources.Limits should be nil")
	assert.Check(t, pod.Spec.Containers[0].Resources.Requests != nil, "Containers[0].Resources.Requests should be nil")
	assert.Check(t, is.Equal(ptrQuantity(resource.MustParse("0.99")).Value(),
		pod.Spec.Containers[0].Resources.Requests.Cpu().Value()), "Containers[0].Resources.Requests.CPU doesn't match")
	assert.Check(t, is.Equal(ptrQuantity(resource.MustParse("1.5G")).Value(),
		pod.Spec.Containers[0].Resources.Requests.Memory().Value()), "Containers[0].Resources.Requests.Memory doesn't match")
}

// Tests get pod with GPU.
func TestGetPodWithGPU(t *testing.T) {
	_, aciServerMocker, provider, err := prepareMocks()

	if err != nil {
		t.Fatal("Unable to prepare the mocks", err)
	}

	podName := "pod-" + uuid.New().String()
	podNamespace := "ns-" + uuid.New().String()

	aciServerMocker.OnGetContainerGroup = func(subscription, resourceGroup, containerGroup string) (int, interface{}) {
		assert.Equal(t, fakeSubscription, subscription, "Subscription doesn't match")
		assert.Equal(t, fakeResourceGroup, resourceGroup, "Resource group doesn't match")
		assert.Equal(t, podNamespace+"-"+podName, containerGroup, "Container group name is not expected")

		return http.StatusOK, aci.ContainerGroup{
			Tags: map[string]string{
				"NodeName": fakeNodeName,
			},
			ContainerGroupProperties: aci.ContainerGroupProperties{
				ProvisioningState: "Creating",
				Containers: []aci.Container{
					aci.Container{
						Name: "nginx",
						ContainerProperties: aci.ContainerProperties{
							Image:   "nginx",
							Command: []string{"nginx", "-g", "daemon off;"},
							Ports: []aci.ContainerPort{
								{
									Protocol: aci.ContainerNetworkProtocolTCP,
									Port:     80,
								},
							},
							Resources: aci.ResourceRequirements{
								Requests: &aci.ComputeResources{
									CPU:        0.99,
									MemoryInGB: 1.5,
									GPU: &aci.GPUResource{
										Count: 5,
										SKU:   aci.P100,
									},
								},
								Limits: &aci.ComputeResources{
									GPU: &aci.GPUResource{
										Count: 5,
										SKU:   aci.P100,
									},
								},
							},
						},
					},
				},
			},
		}
	}

	pod, err := provider.GetPod(context.Background(), podNamespace, podName)
	if err != nil {
		t.Fatal("Failed to get pod", err)
	}

	assert.Check(t, pod != nil, "Response pod should not be nil")
	assert.Check(t, pod.Spec.Containers != nil, "Containers should not be nil")
	assert.Check(t, pod.Spec.Containers[0].Resources.Requests != nil, "Containers[0].Resources.Requests should not be nil")
	assert.Check(
		t,
		is.Equal(ptrQuantity(resource.MustParse("0.99")).Value(), pod.Spec.Containers[0].Resources.Requests.Cpu().Value()),
		"Containers[0].Resources.Requests.CPU doesn't match")
	assert.Check(
		t,
		is.Equal(ptrQuantity(resource.MustParse("1.5G")).Value(), pod.Spec.Containers[0].Resources.Requests.Memory().Value()),
		"Containers[0].Resources.Requests.Memory doesn't match")
	gpuQuantity, ok := pod.Spec.Containers[0].Resources.Requests[gpuResourceName]
	assert.Check(t, is.Equal(ok, true), "Containers[0].Resources.Requests.GPU should not be nil")
	assert.Check(
		t,
		is.Equal(ptrQuantity(resource.MustParse("5")).Value(), ptrQuantity(gpuQuantity).Value()),
		"Containers[0].Resources.Requests.GPU.Count doesn't match")
	assert.Check(t, pod.Spec.Containers[0].Resources.Limits != nil, "Containers[0].Resources.Limits should not be nil")
	gpuQuantity, ok = pod.Spec.Containers[0].Resources.Limits[gpuResourceName]
	assert.Check(t, is.Equal(ok, true), "Containers[0].Resources.Requests.GPU should not be nil")
	assert.Check(
		t,
		is.Equal(ptrQuantity(resource.MustParse("5")).Value(), ptrQuantity(gpuQuantity).Value()),
		"Containers[0].Resources.Limits.GPU.Count doesn't match")
}

func TestGetPodWithContainerID(t *testing.T) {
	_, aciServerMocker, provider, err := prepareMocks()

	if err != nil {
		t.Fatal("Unable to prepare the mocks", err)
	}

	podName := "pod-" + uuid.New().String()
	podNamespace := "ns-" + uuid.New().String()
	containerName := "c-" + uuid.New().String()
	containerImage := "ci-" + uuid.New().String()

	cgID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.ContainerInstance/containerGroups/%s-%s", fakeSubscription, fakeResourceGroup, podNamespace, podName)

	aciServerMocker.OnGetContainerGroup = func(subscription, resourceGroup, containerGroup string) (int, interface{}) {
		assert.Check(t, is.Equal(fakeSubscription, subscription), "Subscription doesn't match")
		assert.Check(t, is.Equal(fakeResourceGroup, resourceGroup), "Resource group doesn't match")
		assert.Check(t, is.Equal(podNamespace+"-"+podName, containerGroup), "Container group name is not expected")

		return http.StatusOK, aci.ContainerGroup{
			ID: cgID,
			Tags: map[string]string{
				"NodeName": fakeNodeName,
			},
			ContainerGroupProperties: aci.ContainerGroupProperties{
				ProvisioningState: "Creating",
				Containers: []aci.Container{
					aci.Container{
						Name: containerName,
						ContainerProperties: aci.ContainerProperties{
							Image:   containerImage,
							Command: []string{"nginx", "-g", "daemon off;"},
							Ports: []aci.ContainerPort{
								{
									Protocol: aci.ContainerNetworkProtocolTCP,
									Port:     80,
								},
							},
							Resources: aci.ResourceRequirements{
								Requests: &aci.ComputeResources{
									CPU:        0.99,
									MemoryInGB: 1.5,
								},
							},
						},
					},
				},
			},
		}
	}

	pod, err := provider.GetPod(context.Background(), podNamespace, podName)
	if err != nil {
		t.Fatal("Failed to get pod", err)
	}

	assert.Check(t, pod != nil, "Response pod should not be nil")
	assert.Check(t, is.Equal(1, len(pod.Status.ContainerStatuses)), "1 container status is expected")
	assert.Check(t, is.Equal(containerName, pod.Status.ContainerStatuses[0].Name), "Container name in the container status doesn't match")
	assert.Check(t, is.Equal(containerImage, pod.Status.ContainerStatuses[0].Image), "Container image in the container status doesn't match")
	assert.Check(t, is.Equal(getContainerID(cgID, containerName), pod.Status.ContainerStatuses[0].ContainerID), "Container ID in the container status is not expected")
}

func TestPodToACISecretEnvVar(t *testing.T) {

	testKey := "testVar"
	testVal := "testVal"

	e := v1.EnvVar{
		Name:  testKey,
		Value: testVal,
		ValueFrom: &v1.EnvVarSource{
			SecretKeyRef: &v1.SecretKeySelector{},
		},
	}
	aciEnvVar := getACIEnvVar(e)

	if aciEnvVar.Value != "" {
		t.Fatalf("ACI Env Variable Value should be empty for a secret")
	}

	if aciEnvVar.Name != testKey {
		t.Fatalf("ACI Env Variable Name does not match expected Name")
	}

	if aciEnvVar.SecureValue != testVal {
		t.Fatalf("ACI Env Variable Secure Value does not match expected value")
	}
}

func TestPodToACIEnvVar(t *testing.T) {

	testKey := "testVar"
	testVal := "testVal"

	e := v1.EnvVar{
		Name:      testKey,
		Value:     testVal,
		ValueFrom: &v1.EnvVarSource{},
	}
	aciEnvVar := getACIEnvVar(e)

	if aciEnvVar.SecureValue != "" {
		t.Fatalf("ACI Env Variable Secure Value should be empty for non-secret variables")
	}

	if aciEnvVar.Name != testKey {
		t.Fatalf("ACI Env Variable Name does not match expected Name")
	}

	if aciEnvVar.Value != testVal {
		t.Fatalf("ACI Env Variable Value does not match expected value")
	}
}

func prepareMocks() (*AADMock, *ACIMock, *ACIProvider, error) {
	aadServerMocker := NewAADMock()
	aciServerMocker := NewACIMock()

	aciServerMocker.OnGetRPManifest = func() (int, interface{}) {
		manifest := &aci.ResourceProviderManifest{
			Metadata: &aci.ResourceProviderMetadata{
				GPURegionalSKUs: []*aci.GPURegionalSKU{
					&aci.GPURegionalSKU{
						Location: fakeRegion,
						SKUs:     []aci.GPUSKU{aci.K80, aci.P100, aci.V100},
					},
				},
			},
		}

		return http.StatusOK, manifest
	}

	provider, err := createTestProvider(aadServerMocker, aciServerMocker, nil)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create the test provider %s", err.Error())
	}

	return aadServerMocker, aciServerMocker, provider, nil
}

func createTestProvider(aadServerMocker *AADMock, aciServerMocker *ACIMock, resourceManager *manager.ResourceManager) (*ACIProvider, error) {
	auth := azure.NewAuthentication(
		azure.PublicCloud.Name,
		fakeClientID,
		fakeClientSecret,
		fakeSubscription,
		fakeTenantID,
		fakeUserIdentity)

	auth.ActiveDirectoryEndpoint = aadServerMocker.GetServerURL()
	auth.ResourceManagerEndpoint = aciServerMocker.GetServerURL()

	file, err := ioutil.TempFile("", "auth.json")
	if err != nil {
		return nil, err
	}

	defer os.Remove(file.Name())

	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(auth)

	if _, err := file.Write(b.Bytes()); err != nil {
		return nil, err
	}

	os.Setenv("AZURE_AUTH_LOCATION", file.Name())
	os.Setenv("ACI_RESOURCE_GROUP", fakeResourceGroup)
	os.Setenv("ACI_REGION", fakeRegion)

	if resourceManager == nil {
		resourceManager, err = manager.NewResourceManager(nil, nil, nil, nil, nil, nil)

		if err != nil {
			return nil, err
		}
	}

	provider, err := NewACIProvider("example.toml", resourceManager, fakeNodeName, "Linux", "0.0.0.0", 10250, "cluster.local")
	if err != nil {
		return nil, err
	}

	return provider, nil
}

func ptrQuantity(q resource.Quantity) *resource.Quantity {
	return &q
}

func TestConfigureNode(t *testing.T) {
	_, _, provider, err := prepareMocks()
	if err != nil {
		t.Fatal("Unable to prepare the mocks", err)
	}

	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "virtual-kubelet",
			Labels: map[string]string{
				"type":                   "virtual-kubelet",
				"kubernetes.io/role":     "agent",
				"kubernetes.io/hostname": "virtual-kubelet",
			},
		},
		Spec: v1.NodeSpec{},
		Status: v1.NodeStatus{
			NodeInfo: v1.NodeSystemInfo{
				Architecture:   "amd64",
				KubeletVersion: "1.18.4",
			},
		},
	}
	provider.ConfigureNode(context.TODO(), node)
	assert.Equal(t, "true", node.ObjectMeta.Labels["alpha.service-controller.kubernetes.io/exclude-balancer"], "exclude-balancer label doesn't match")
	assert.Equal(t, "true", node.ObjectMeta.Labels["node.kubernetes.io/exclude-from-external-load-balancers"], "exclude-from-external-load-balancers label doesn't match")
	assert.Equal(t, "false", node.ObjectMeta.Labels["kubernetes.azure.com/managed"], "kubernetes.azure.com/managed label doesn't match")
}

func TestCreatePodWithNamedLivenessProbe(t *testing.T) {
	_, aciServerMocker, provider, err := prepareMocks()

	if err != nil {
		t.Fatal("Unable to prepare the mocks", err)
	}

	podName := "pod-" + uuid.New().String()
	podNamespace := "ns-" + uuid.New().String()

	aciServerMocker.OnCreate = func(subscription, resourceGroup, containerGroup string, cg *aci.ContainerGroup) (int, interface{}) {
		assert.Check(t, cg.Containers[0].LivenessProbe != nil, "Liveness probe expected")
		assert.Check(t, is.Equal(10, cg.Containers[0].LivenessProbe.InitialDelaySeconds), "Initial Probe Delay doesn't match")
		assert.Check(t, is.Equal(5, cg.Containers[0].LivenessProbe.Period), "Probe Period doesn't match")
		assert.Check(t, is.Equal(60, cg.Containers[0].LivenessProbe.TimeoutSeconds), "Probe Timeout doesn't match")
		assert.Check(t, is.Equal(3, cg.Containers[0].LivenessProbe.SuccessThreshold), "Probe Success Threshold doesn't match")
		assert.Check(t, is.Equal(5, cg.Containers[0].LivenessProbe.FailureThreshold), "Probe Failure Threshold doesn't match")
		assert.Check(t, cg.Containers[0].LivenessProbe.HTTPGet != nil, "Expected an HTTP Get Probe")
		assert.Check(t, is.Equal(8080, cg.Containers[0].LivenessProbe.HTTPGet.Port), "Expected Port to be 8080")
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      podName,
				Namespace: podNamespace,
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					v1.Container{
						Name: "nginx",
						Ports: []v1.ContainerPort{
							v1.ContainerPort{
								Name:          "http",
								ContainerPort: 8080,
							},
						},
						LivenessProbe: &v1.Probe{
							Handler: v1.Handler{
								HTTPGet: &v1.HTTPGetAction{
									Port: intstr.FromString("http"),
									Path: "/",
								},
							},
							InitialDelaySeconds: 10,
							PeriodSeconds:       5,
							TimeoutSeconds:      60,
							SuccessThreshold:    3,
							FailureThreshold:    5,
						},
					},
				},
			},
		}

		if err := provider.CreatePod(context.Background(), pod); err != nil {
			t.Fatal("Failed to create pod", err)
		}

		return http.StatusOK, cg
	}
}

func TestCreatePodWithLivenessProbe(t *testing.T) {
	_, aciServerMocker, provider, err := prepareMocks()

	if err != nil {
		t.Fatal("Unable to prepare the mocks", err)
	}

	podName := "pod-" + uuid.New().String()
	podNamespace := "ns-" + uuid.New().String()

	aciServerMocker.OnCreate = func(subscription, resourceGroup, containerGroup string, cg *aci.ContainerGroup) (int, interface{}) {
		assert.Check(t, is.Equal(fakeSubscription, subscription), "Subscription doesn't match")
		assert.Check(t, is.Equal(fakeResourceGroup, resourceGroup), "Resource group doesn't match")
		assert.Check(t, cg != nil, "Container group is nil")
		assert.Check(t, is.Equal(podNamespace+"-"+podName, containerGroup), "Container group name is not expected")
		assert.Check(t, cg.ContainerGroupProperties.Containers != nil, "Containers should not be nil")
		assert.Check(t, is.Equal(1, len(cg.ContainerGroupProperties.Containers)), "1 Container is expected")
		assert.Check(t, is.Equal("nginx", cg.ContainerGroupProperties.Containers[0].Name), "Container nginx is expected")
		assert.Check(t, cg.Containers[0].LivenessProbe != nil, "Liveness probe expected")
		assert.Check(t, is.Equal(int32(10), cg.Containers[0].LivenessProbe.InitialDelaySeconds), "Initial Probe Delay doesn't match")
		assert.Check(t, is.Equal(int32(5), cg.Containers[0].LivenessProbe.Period), "Probe Period doesn't match")
		assert.Check(t, is.Equal(int32(60), cg.Containers[0].LivenessProbe.TimeoutSeconds), "Probe Timeout doesn't match")
		assert.Check(t, is.Equal(int32(3), cg.Containers[0].LivenessProbe.SuccessThreshold), "Probe Success Threshold doesn't match")
		assert.Check(t, is.Equal(int32(5), cg.Containers[0].LivenessProbe.FailureThreshold), "Probe Failure Threshold doesn't match")
		assert.Check(t, cg.Containers[0].LivenessProbe.HTTPGet != nil, "Expected an HTTP Get Probe")

		return http.StatusOK, cg
	}

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: podNamespace,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				v1.Container{
					Name: "nginx",
					LivenessProbe: &v1.Probe{
						Handler: v1.Handler{
							HTTPGet: &v1.HTTPGetAction{
								Port: intstr.FromInt(8080),
								Path: "/",
							},
						},
						InitialDelaySeconds: 10,
						PeriodSeconds:       5,
						TimeoutSeconds:      60,
						SuccessThreshold:    3,
						FailureThreshold:    5,
					},
				},
			},
		},
	}

	if err := provider.CreatePod(context.Background(), pod); err != nil {
		t.Fatal("Failed to create pod", err)
	}
}

func TestCreatePodWithReadinessProbe(t *testing.T) {
	_, aciServerMocker, provider, err := prepareMocks()

	if err != nil {
		t.Fatal("Unable to prepare the mocks", err)
	}

	podName := "pod-" + uuid.New().String()
	podNamespace := "ns-" + uuid.New().String()

	aciServerMocker.OnCreate = func(subscription, resourceGroup, containerGroup string, cg *aci.ContainerGroup) (int, interface{}) {
		assert.Check(t, is.Equal(fakeSubscription, subscription), "Subscription doesn't match")
		assert.Check(t, is.Equal(fakeResourceGroup, resourceGroup), "Resource group doesn't match")
		assert.Check(t, cg != nil, "Container group is nil")
		assert.Check(t, is.Equal(podNamespace+"-"+podName, containerGroup), "Container group name is not expected")
		assert.Check(t, cg.ContainerGroupProperties.Containers != nil, "Containers should not be nil")
		assert.Check(t, is.Equal(1, len(cg.ContainerGroupProperties.Containers)), "1 Container is expected")
		assert.Check(t, is.Equal("nginx", cg.ContainerGroupProperties.Containers[0].Name), "Container nginx is expected")
		assert.Check(t, cg.Containers[0].ReadinessProbe != nil, "Readiness probe expected")
		assert.Check(t, is.Equal(int32(10), cg.Containers[0].ReadinessProbe.InitialDelaySeconds), "Initial Probe Delay doesn't match")
		assert.Check(t, is.Equal(int32(5), cg.Containers[0].ReadinessProbe.Period), "Probe Period doesn't match")
		assert.Check(t, is.Equal(int32(60), cg.Containers[0].ReadinessProbe.TimeoutSeconds), "Probe Timeout doesn't match")
		assert.Check(t, is.Equal(int32(3), cg.Containers[0].ReadinessProbe.SuccessThreshold), "Probe Success Threshold doesn't match")
		assert.Check(t, is.Equal(int32(5), cg.Containers[0].ReadinessProbe.FailureThreshold), "Probe Failure Threshold doesn't match")
		assert.Check(t, cg.Containers[0].ReadinessProbe.HTTPGet != nil, "Expected an HTTP Get Probe")

		return http.StatusOK, cg
	}

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: podNamespace,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				v1.Container{
					Name: "nginx",
					ReadinessProbe: &v1.Probe{
						Handler: v1.Handler{
							HTTPGet: &v1.HTTPGetAction{
								Port: intstr.FromInt(8080),
								Path: "/",
							},
						},
						InitialDelaySeconds: 10,
						PeriodSeconds:       5,
						TimeoutSeconds:      60,
						SuccessThreshold:    3,
						FailureThreshold:    5,
					},
				},
			},
		},
	}

	if err := provider.CreatePod(context.Background(), pod); err != nil {
		t.Fatal("Failed to create pod", err)
	}
}

func TestCreatePodWithProjectedVolume(t *testing.T) {
	podName := "pod-" + uuid.New().String()
	podNamespace := "ns-" + uuid.New().String()

	aadServerMocker := NewAADMock()
	aciServerMocker := NewACIMock()

	aciServerMocker.OnGetRPManifest = func() (int, interface{}) {
		manifest := &aci.ResourceProviderManifest{
			Metadata: &aci.ResourceProviderMetadata{
				GPURegionalSKUs: []*aci.GPURegionalSKU{
					&aci.GPURegionalSKU{
						Location: fakeRegion,
						SKUs:     []aci.GPUSKU{aci.K80, aci.P100, aci.V100},
					},
				},
			},
		}

		return http.StatusOK, manifest
	}

	mockCtrl := gomock.NewController(GinkgoT())
	podLister := NewMockPodLister(mockCtrl)
	secretLister := NewMockSecretLister(mockCtrl)
	configMapLister := NewMockConfigMapLister(mockCtrl)
	serviceLister := NewMockServiceLister(mockCtrl)
	pvcLister := NewMockPersistentVolumeClaimLister(mockCtrl)
	pvLister := NewMockPersistentVolumeLister(mockCtrl)

	resourceManager, err := manager.NewResourceManager(podLister, secretLister, configMapLister, serviceLister, pvcLister, pvLister)
	if err != nil {
		t.Fatal("Unable to prepare the mocks for resourceManager", err)
	}

	configMapNamespaceLister := NewMockConfigMapNamespaceLister(mockCtrl)
	configMapLister.EXPECT().ConfigMaps(podNamespace).Return(configMapNamespaceLister)
	configMapNamespaceLister.EXPECT().Get("kube-root-ca.crt").Return(&v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kube-root-ca.crt",
		},
		Data: map[string]string{
			"ca.crt": "fake-ca-data",
			"foo":    "bar",
		},
	}, nil)

	provider, err := createTestProvider(aadServerMocker, aciServerMocker, resourceManager)
	if err != nil {
		t.Fatal("Unable to create test provider", err)
	}

	aciServerMocker.OnCreate = func(subscription, resourceGroup, containerGroup string, cg *aci.ContainerGroup) (int, interface{}) {
		assert.Check(t, is.Equal(fakeSubscription, subscription), "Subscription doesn't match")
		assert.Check(t, is.Equal(fakeResourceGroup, resourceGroup), "Resource group doesn't match")
		assert.Check(t, cg != nil, "Container group is nil")
		assert.Check(t, is.Equal(podNamespace+"-"+podName, containerGroup), "Container group name is not expected")
		assert.Check(t, cg.ContainerGroupProperties.Containers != nil, "Containers should not be nil")
		assert.Check(t, is.Equal(1, len(cg.ContainerGroupProperties.Containers)), "1 Container is expected")
		assert.Check(t, is.Equal("nginx", cg.ContainerGroupProperties.Containers[0].Name), "Container nginx is expected")
		assert.Check(t, is.Equal(1, len(cg.Volumes)), "volume count not match")
		assert.Check(t, is.Equal("projectedvolume", cg.Volumes[0].Name), "volume name doesn't match")
		assert.Check(t, is.Equal(base64.StdEncoding.EncodeToString([]byte("fake-ca-data")), cg.Volumes[0].Secret["ca.crt"]), "configmap data doesn't match")

		return http.StatusOK, cg
	}

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: podNamespace,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				v1.Container{
					Name: "nginx",
					ReadinessProbe: &v1.Probe{
						Handler: v1.Handler{
							HTTPGet: &v1.HTTPGetAction{
								Port: intstr.FromInt(8080),
								Path: "/",
							},
						},
						InitialDelaySeconds: 10,
						PeriodSeconds:       5,
						TimeoutSeconds:      60,
						SuccessThreshold:    3,
						FailureThreshold:    5,
					},
				},
			},
			Volumes: []v1.Volume{
				{
					Name: "projectedvolume",
					VolumeSource: v1.VolumeSource{
						Projected: &v1.ProjectedVolumeSource{
							Sources: []v1.VolumeProjection{
								{
									ConfigMap: &v1.ConfigMapProjection{
										LocalObjectReference: v1.LocalObjectReference{
											Name: "kube-root-ca.crt",
										},
										Items: []v1.KeyToPath{
											{
												Key:  "ca.crt",
												Path: "ca.crt",
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

	if err := provider.CreatePod(context.Background(), pod); err != nil {
		t.Fatal("Failed to create pod", err)
	}
}

func TestCreatePodWithCSIVolume(t *testing.T) {
	podName := "pod-name"
	podNamespace := "ns-name"
	fakeVolumeSecret := "fake-volume-secret"
	fakeShareName := "aksshare"
	azureFileVolumeName := "azure"

	aadServerMocker := NewAADMock()
	aciServerMocker := NewACIMock()
	mockCtrl := gomock.NewController(GinkgoT())
	mockPodLister := NewMockPodLister(mockCtrl)
	mockSecretLister := NewMockSecretLister(mockCtrl)
	mockSecretNamespaceLister := NewMockSecretNamespaceLister(mockCtrl)
	mockConfigMapLister := NewMockConfigMapLister(mockCtrl)
	mockServiceLister := NewMockServiceLister(mockCtrl)
	mockPvcLister := NewMockPersistentVolumeClaimLister(mockCtrl)
	mockPvLister := NewMockPersistentVolumeLister(mockCtrl)

	aciServerMocker.OnGetRPManifest = func() (int, interface{}) {
		manifest := &aci.ResourceProviderManifest{
			Metadata: &aci.ResourceProviderMetadata{
				GPURegionalSKUs: []*aci.GPURegionalSKU{
					{
						Location: fakeRegion,
						SKUs:     []aci.GPUSKU{aci.K80, aci.P100, aci.V100},
					},
				},
			},
		}
		return http.StatusOK, manifest
	}

	aciServerMocker.OnCreate = func(subscription, resourceGroup, containerGroup string, cg *aci.ContainerGroup) (int, interface{}) {
		assert.Check(t, is.Equal(fakeSubscription, subscription), "Subscription doesn't match")
		assert.Check(t, is.Equal(fakeResourceGroup, resourceGroup), "Resource group doesn't match")
		assert.Check(t, cg != nil, "Container group is nil")
		assert.Check(t, is.Equal(podNamespace+"-"+podName, containerGroup), "Container group name is not expected")
		assert.Check(t, cg.ContainerGroupProperties.Containers != nil, "Containers should not be nil")
		assert.Check(t, is.Equal(1, len(cg.ContainerGroupProperties.Containers)), "1 Container is expected")
		assert.Check(t, is.Equal("nginx", cg.ContainerGroupProperties.Containers[0].Name), "Container nginx is expected")
		assert.Check(t, is.Equal(1, len(cg.Volumes)), "volume count not match")

		return http.StatusOK, cg
	}

	configMapNamespaceLister := NewMockConfigMapNamespaceLister(mockCtrl)
	mockConfigMapLister.EXPECT().ConfigMaps(podNamespace).Return(configMapNamespaceLister)
	configMapNamespaceLister.EXPECT().Get("kube-root-ca.crt").Return(&v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kube-root-ca.crt",
		},
		Data: map[string]string{
			"ca.crt": "fake-ca-data",
			"foo":    "bar",
		},
	}, nil)

	fakeSecret := v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fakeVolumeSecret,
			Namespace: podNamespace,
		},
		Data: map[string][]byte{
			azureFileStorageAccountName: []byte("azure storage account name"),
			azureFileStorageAccountKey:  []byte("azure storage account key")},
	}

	fakeVolumeMount := v1.VolumeMount{
		Name:      azureFileVolumeName,
		MountPath: "/mnt/azure",
	}

	fakePodVolume := v1.Volume{
		Name: azureFileVolumeName,
		VolumeSource: v1.VolumeSource{
			CSI: &v1.CSIVolumeSource{
				Driver: "file.csi.azure.com",
				VolumeAttributes: map[string]string{
					azureFileSecretName: fakeVolumeSecret,
					azureFileShareName:  fakeShareName,
				},
			},
		},
	}

	cases := []struct {
		description   string
		secretVolume  *v1.Secret
		volume        v1.Volume
		expectedError error
	}{
		{
			description:   "Secret is nil",
			secretVolume:  nil,
			volume:        fakePodVolume,
			expectedError: fmt.Errorf("the secret %s for AzureFile CSI driver %s is not found", fakeSecret.Name, fakePodVolume.Name),
		},
		{
			description:   "Volume has a secret with a valid value",
			secretVolume:  &fakeSecret,
			volume:        fakePodVolume,
			expectedError: nil,
		},
		{
			description:  "Volume has no secret",
			secretVolume: &fakeSecret,
			volume: v1.Volume{
				Name: azureFileVolumeName,
				VolumeSource: v1.VolumeSource{
					CSI: &v1.CSIVolumeSource{
						Driver:           "file.csi.azure.com",
						VolumeAttributes: map[string]string{},
					},
				}},
			expectedError: fmt.Errorf("secret volume attribute for AzureFile CSI driver %s cannot be empty or nil", azureFileVolumeName),
		},
		{
			description:  "Volume has no share name",
			secretVolume: &fakeSecret,
			volume: v1.Volume{
				Name: azureFileVolumeName,
				VolumeSource: v1.VolumeSource{
					CSI: &v1.CSIVolumeSource{
						Driver: "file.csi.azure.com",
						VolumeAttributes: map[string]string{
							azureFileSecretName: fakeVolumeSecret,
						},
					},
				}},
			expectedError: fmt.Errorf("share name for AzureFile CSI driver %s cannot be empty or nil", fakePodVolume.Name),
		},
		{
			description:  "Volume is Disk Driver",
			secretVolume: &fakeSecret,
			volume: v1.Volume{
				Name: azureFileVolumeName,
				VolumeSource: v1.VolumeSource{
					CSI: &v1.CSIVolumeSource{
						Driver: "disk.csi.azure.com",
						VolumeAttributes: map[string]string{
							azureFileSecretName: fakeVolumeSecret,
							azureFileShareName:  fakeShareName,
						},
					},
				}},
			expectedError: fmt.Errorf("pod %s requires volume %s which is of an unsupported type %s", podName, azureFileVolumeName, "disk.csi.azure.com"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {

			resourceManager, err := manager.NewResourceManager(mockPodLister, mockSecretLister, mockConfigMapLister, mockServiceLister, mockPvcLister, mockPvLister)
			if err != nil {
				t.Fatal("Unable to prepare the mocks for resourceManager", err)
			}

			provider, err := createTestProvider(aadServerMocker, aciServerMocker, resourceManager)
			if err != nil {
				t.Fatal("Unable to create test provider", err)
			}

			pod := &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      podName,
					Namespace: podNamespace,
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "nginx",
							ReadinessProbe: &v1.Probe{
								Handler: v1.Handler{
									HTTPGet: &v1.HTTPGetAction{
										Port: intstr.FromInt(8080),
										Path: "/",
									},
								},
								InitialDelaySeconds: 10,
								PeriodSeconds:       5,
								TimeoutSeconds:      60,
								SuccessThreshold:    3,
								FailureThreshold:    5,
							},
						},
					},
				},
			}

			pod.Spec.Containers[0].VolumeMounts = append(pod.Spec.Containers[0].VolumeMounts, fakeVolumeMount)

			if tc.volume.CSI.VolumeAttributes != nil && len(tc.volume.CSI.VolumeAttributes) != 0 {
				mockSecretLister.EXPECT().Secrets(podNamespace).Return(mockSecretNamespaceLister)
				mockSecretNamespaceLister.EXPECT().Get(tc.volume.CSI.VolumeAttributes[azureFileSecretName]).Return(tc.secretVolume, nil)
			}

			pod.Spec.Volumes = append(pod.Spec.Volumes, tc.volume)

			err = provider.CreatePod(context.Background(), pod)

			if tc.expectedError != nil {
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				assert.NilError(t, tc.expectedError, err)
			}
		})
	}
}
