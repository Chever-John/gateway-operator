/*
Copyright 2022 Kong Inc.

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

// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	"context"

	v1alpha1 "github.com/kong/gateway-operator/apis/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeAIGateways implements AIGatewayInterface
type FakeAIGateways struct {
	Fake *FakeApisV1alpha1
	ns   string
}

var aigatewaysResource = v1alpha1.SchemeGroupVersion.WithResource("aigateways")

var aigatewaysKind = v1alpha1.SchemeGroupVersion.WithKind("AIGateway")

// Get takes name of the aIGateway, and returns the corresponding aIGateway object, and an error if there is any.
func (c *FakeAIGateways) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha1.AIGateway, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(aigatewaysResource, c.ns, name), &v1alpha1.AIGateway{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.AIGateway), err
}

// List takes label and field selectors, and returns the list of AIGateways that match those selectors.
func (c *FakeAIGateways) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha1.AIGatewayList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(aigatewaysResource, aigatewaysKind, c.ns, opts), &v1alpha1.AIGatewayList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.AIGatewayList{ListMeta: obj.(*v1alpha1.AIGatewayList).ListMeta}
	for _, item := range obj.(*v1alpha1.AIGatewayList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested aIGateways.
func (c *FakeAIGateways) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(aigatewaysResource, c.ns, opts))

}

// Create takes the representation of a aIGateway and creates it.  Returns the server's representation of the aIGateway, and an error, if there is any.
func (c *FakeAIGateways) Create(ctx context.Context, aIGateway *v1alpha1.AIGateway, opts v1.CreateOptions) (result *v1alpha1.AIGateway, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(aigatewaysResource, c.ns, aIGateway), &v1alpha1.AIGateway{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.AIGateway), err
}

// Update takes the representation of a aIGateway and updates it. Returns the server's representation of the aIGateway, and an error, if there is any.
func (c *FakeAIGateways) Update(ctx context.Context, aIGateway *v1alpha1.AIGateway, opts v1.UpdateOptions) (result *v1alpha1.AIGateway, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(aigatewaysResource, c.ns, aIGateway), &v1alpha1.AIGateway{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.AIGateway), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeAIGateways) UpdateStatus(ctx context.Context, aIGateway *v1alpha1.AIGateway, opts v1.UpdateOptions) (*v1alpha1.AIGateway, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(aigatewaysResource, "status", c.ns, aIGateway), &v1alpha1.AIGateway{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.AIGateway), err
}

// Delete takes name of the aIGateway and deletes it. Returns an error if one occurs.
func (c *FakeAIGateways) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteActionWithOptions(aigatewaysResource, c.ns, name, opts), &v1alpha1.AIGateway{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeAIGateways) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(aigatewaysResource, c.ns, listOpts)

	_, err := c.Fake.Invokes(action, &v1alpha1.AIGatewayList{})
	return err
}

// Patch applies the patch and returns the patched aIGateway.
func (c *FakeAIGateways) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.AIGateway, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(aigatewaysResource, c.ns, name, pt, data, subresources...), &v1alpha1.AIGateway{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.AIGateway), err
}