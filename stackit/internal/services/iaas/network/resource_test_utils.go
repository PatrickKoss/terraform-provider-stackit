package network

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stackitcloud/stackit-sdk-go/core/utils"
	"github.com/stackitcloud/stackit-sdk-go/services/iaas"
	"github.com/stackitcloud/stackit-sdk-go/services/iaasalpha"
	"github.com/stackitcloud/terraform-provider-stackit/stackit/internal/core"
	mock_network_v1 "github.com/stackitcloud/terraform-provider-stackit/stackit/internal/services/iaas/network/mock/v1"
	mock_network_v2 "github.com/stackitcloud/terraform-provider-stackit/stackit/internal/services/iaas/network/mock/v2"
	networkModel "github.com/stackitcloud/terraform-provider-stackit/stackit/internal/services/iaas/network/utils/model"
	"go.uber.org/mock/gomock"
)

// TestContext holds common test setup for v1 (non-experimental)
type TestContext struct {
	T          *testing.T
	MockCtrl   *gomock.Controller
	MockClient *mock_network_v1.MockDefaultApi
	Resource   *networkResource
	Ctx        context.Context
	CancelFunc context.CancelFunc
}

// TestContextAlpha holds common test setup for v2 (experimental/alpha)
type TestContextAlpha struct {
	T               *testing.T
	MockCtrl        *gomock.Controller
	MockClientAlpha *mock_network_v2.MockDefaultApi
	Resource        *networkResource
	Ctx             context.Context
	CancelFunc      context.CancelFunc
}

// NewTestContext creates a new test context with mock client for v1 API
func NewTestContext(t *testing.T) *TestContext {
	ctrl := gomock.NewController(t)
	mockClient := mock_network_v1.NewMockDefaultApi(ctrl)

	resource := &networkResource{
		client:         mockClient,
		isExperimental: false,
		providerData: core.ProviderData{
			Region: "eu01",
		},
	}

	return &TestContext{
		T:          t,
		MockCtrl:   ctrl,
		MockClient: mockClient,
		Resource:   resource,
		Ctx:        context.Background(),
	}
}

// NewTestContextAlpha creates a new test context with mock client for v2 alpha API
func NewTestContextAlpha(t *testing.T) *TestContextAlpha {
	ctrl := gomock.NewController(t)
	mockClientAlpha := mock_network_v2.NewMockDefaultApi(ctrl)

	resource := &networkResource{
		alphaClient:    mockClientAlpha,
		isExperimental: true,
		providerData: core.ProviderData{
			Region: "eu01",
		},
	}

	return &TestContextAlpha{
		T:               t,
		MockCtrl:        ctrl,
		MockClientAlpha: mockClientAlpha,
		Resource:        resource,
		Ctx:             context.Background(),
	}
}

// Close cleans up the test context
func (tc *TestContext) Close() {
	if tc.CancelFunc != nil {
		tc.CancelFunc()
	}
	tc.MockCtrl.Finish()
}

// Close cleans up the alpha test context
func (tc *TestContextAlpha) Close() {
	if tc.CancelFunc != nil {
		tc.CancelFunc()
	}
	tc.MockCtrl.Finish()
}

// GetSchema returns the resource schema
func (tc *TestContext) GetSchema() resource.SchemaResponse {
	schemaResp := resource.SchemaResponse{}
	tc.Resource.Schema(tc.Ctx, resource.SchemaRequest{}, &schemaResp)
	return schemaResp
}

// GetSchema returns the resource schema for alpha
func (tc *TestContextAlpha) GetSchema() resource.SchemaResponse {
	schemaResp := resource.SchemaResponse{}
	tc.Resource.Schema(tc.Ctx, resource.SchemaRequest{}, &schemaResp)
	return schemaResp
}

// BuildNetwork creates a iaas.Network with the given fields
func BuildNetwork(networkId, name, gateway, publicIP string, prefixes, nameservers []string) *iaas.Network {
	return &iaas.Network{
		NetworkId:   utils.Ptr(networkId),
		Name:        utils.Ptr(name),
		Gateway:     iaas.NewNullableString(utils.Ptr(gateway)),
		PublicIp:    utils.Ptr(publicIP),
		Prefixes:    &prefixes,
		Nameservers: &nameservers,
		Labels:      &map[string]interface{}{},
		Routed:      utils.Ptr(false),
		State:       utils.Ptr("CREATED"),
	}
}

// BuildNetworkAlpha creates a iaasalpha.Network with the given fields
func BuildNetworkAlpha(networkId, name, gateway, publicIP string, prefixes, nameservers []string) *iaasalpha.Network {
	ipv4 := &iaasalpha.NetworkIPv4{
		Gateway:     iaasalpha.NewNullableString(utils.Ptr(gateway)),
		PublicIp:    utils.Ptr(publicIP),
		Prefixes:    &prefixes,
		Nameservers: &nameservers,
	}

	return &iaasalpha.Network{
		Id:     utils.Ptr(networkId),
		Name:   utils.Ptr(name),
		Ipv4:   ipv4,
		Labels: &map[string]interface{}{},
		Routed: utils.Ptr(false),
		Status: utils.Ptr("CREATED"),
	}
}

// CreateTestModel creates a test model with common values
func CreateTestModel(projectId, networkId, name, ipv4Prefix string) networkModel.Model {
	return networkModel.Model{
		Id:               types.StringValue(projectId + "," + networkId),
		ProjectId:        types.StringValue(projectId),
		NetworkId:        types.StringValue(networkId),
		Name:             types.StringValue(name),
		Nameservers:      types.ListNull(types.StringType),
		IPv4Gateway:      types.StringNull(),
		IPv4Nameservers:  types.ListNull(types.StringType),
		IPv4Prefix:       types.StringValue(ipv4Prefix),
		IPv4PrefixLength: types.Int64Null(),
		Prefixes:         types.ListNull(types.StringType),
		IPv4Prefixes:     types.ListNull(types.StringType),
		IPv6Gateway:      types.StringNull(),
		IPv6Nameservers:  types.ListNull(types.StringType),
		IPv6Prefix:       types.StringNull(),
		IPv6PrefixLength: types.Int64Null(),
		IPv6Prefixes:     types.ListNull(types.StringType),
		PublicIP:         types.StringNull(),
		Labels:           types.MapNull(types.StringType),
		Routed:           types.BoolNull(),
		NoIPv4Gateway:    types.BoolValue(false),
		NoIPv6Gateway:    types.BoolValue(false),
		Region:           types.StringNull(),
		RoutingTableID:   types.StringNull(),
	}
}

// CreateTestModelAlpha creates a test model with region for alpha API
func CreateTestModelAlpha(projectId, networkId, name, region, ipv4Prefix string) networkModel.Model {
	return networkModel.Model{
		Id:               types.StringValue(projectId + "," + region + "," + networkId),
		ProjectId:        types.StringValue(projectId),
		NetworkId:        types.StringValue(networkId),
		Name:             types.StringValue(name),
		Nameservers:      types.ListNull(types.StringType),
		IPv4Gateway:      types.StringNull(),
		IPv4Nameservers:  types.ListNull(types.StringType),
		IPv4Prefix:       types.StringValue(ipv4Prefix),
		IPv4PrefixLength: types.Int64Null(),
		Prefixes:         types.ListNull(types.StringType),
		IPv4Prefixes:     types.ListNull(types.StringType),
		IPv6Gateway:      types.StringNull(),
		IPv6Nameservers:  types.ListNull(types.StringType),
		IPv6Prefix:       types.StringNull(),
		IPv6PrefixLength: types.Int64Null(),
		IPv6Prefixes:     types.ListNull(types.StringType),
		PublicIP:         types.StringNull(),
		Labels:           types.MapNull(types.StringType),
		Routed:           types.BoolNull(),
		NoIPv4Gateway:    types.BoolValue(false),
		NoIPv6Gateway:    types.BoolValue(false),
		Region:           types.StringValue(region),
		RoutingTableID:   types.StringNull(),
	}
}

// CreateRequest creates a test Create request
func CreateRequest(ctx context.Context, schema resource.SchemaResponse, model networkModel.Model) resource.CreateRequest {
	req := resource.CreateRequest{}
	req.Plan = tfsdk.Plan{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	req.Plan.Set(ctx, model)
	return req
}

// CreateResponse creates a test Create response
func CreateResponse(schema resource.SchemaResponse) *resource.CreateResponse {
	resp := &resource.CreateResponse{}
	resp.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	return resp
}

// UpdateRequest creates a test Update request
func UpdateRequest(ctx context.Context, schema resource.SchemaResponse, currentState, plannedState networkModel.Model) resource.UpdateRequest {
	req := resource.UpdateRequest{}
	req.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	req.Plan = tfsdk.Plan{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	req.State.Set(ctx, currentState)
	req.Plan.Set(ctx, plannedState)
	return req
}

// UpdateResponse creates a test Update response
func UpdateResponse(ctx context.Context, schema resource.SchemaResponse, currentState *networkModel.Model) *resource.UpdateResponse {
	resp := &resource.UpdateResponse{}
	resp.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	if currentState != nil {
		resp.State.Set(ctx, *currentState)
	}
	return resp
}

// DeleteRequest creates a test Delete request
func DeleteRequest(ctx context.Context, schema resource.SchemaResponse, state networkModel.Model) resource.DeleteRequest {
	req := resource.DeleteRequest{}
	req.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	req.State.Set(ctx, state)
	return req
}

// DeleteResponse creates a test Delete response
func DeleteResponse(ctx context.Context, schema resource.SchemaResponse, currentState *networkModel.Model) *resource.DeleteResponse {
	resp := &resource.DeleteResponse{}
	resp.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	if currentState != nil {
		resp.State.Set(ctx, *currentState)
	}
	return resp
}

// ReadRequest creates a test Read request
func ReadRequest(ctx context.Context, schema resource.SchemaResponse, state networkModel.Model) resource.ReadRequest {
	req := resource.ReadRequest{}
	req.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	req.State.Set(ctx, state)
	return req
}

// ReadResponse creates a test Read response
func ReadResponse(schema resource.SchemaResponse) *resource.ReadResponse {
	resp := &resource.ReadResponse{}
	resp.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	return resp
}
