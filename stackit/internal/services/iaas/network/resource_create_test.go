package network

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stackitcloud/stackit-sdk-go/core/oapierror"
	"github.com/stackitcloud/stackit-sdk-go/core/utils"
	"github.com/stackitcloud/stackit-sdk-go/services/iaas"
	"github.com/stackitcloud/stackit-sdk-go/services/iaasalpha"
	mock_network_v1 "github.com/stackitcloud/terraform-provider-stackit/stackit/internal/services/iaas/network/mock/v1"
	mock_network_v2 "github.com/stackitcloud/terraform-provider-stackit/stackit/internal/services/iaas/network/mock/v2"
	networkModel "github.com/stackitcloud/terraform-provider-stackit/stackit/internal/services/iaas/network/utils/model"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestCreate_V1_ContextCanceledDuringWait(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	networkId := "network-abc-123"
	networkName := "my-test-network"
	ipv4Prefix := "10.0.0.0/24"
	gateway := "10.0.0.1"
	publicIP := "1.2.3.4"
	prefixes := []string{ipv4Prefix}
	nameservers := []string{"8.8.8.8"}

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	tc.Ctx = ctx

	// Mock CreateNetwork
	mockCreateReq := mock_network_v1.NewMockApiCreateNetworkRequest(tc.MockCtrl)
	mockCreateReq.EXPECT().
		CreateNetworkPayload(gomock.Any()).
		Return(mockCreateReq).
		Times(1)
	mockCreateReq.EXPECT().
		Execute().
		Return(&iaas.Network{NetworkId: utils.Ptr(networkId)}, nil).
		Times(1)

	tc.MockClient.EXPECT().
		CreateNetwork(gomock.Any(), projectId).
		Return(mockCreateReq).
		Times(1)

	// Mock GetNetwork for wait handler
	tc.MockClient.EXPECT().
		GetNetworkExecute(gomock.Any(), projectId, networkId).
		DoAndReturn(func(ctx context.Context, projectId, networkId string) (*iaas.Network, error) {
			time.Sleep(150 * time.Millisecond) // Longer than context timeout
			return nil, ctx.Err()
		}).
		AnyTimes()

	// Prepare request
	schema := tc.GetSchema()
	model := CreateTestModel(projectId, "", networkName, ipv4Prefix)
	req := CreateRequest(tc.Ctx, schema, model)
	resp := CreateResponse(schema)

	// Execute Create
	tc.Resource.Create(tc.Ctx, req, resp)

	// Assertions
	require.True(t, resp.Diagnostics.HasError(), "Expected error due to context timeout")

	var stateAfterCreate networkModel.Model
	diags := resp.State.Get(tc.Ctx, &stateAfterCreate)
	if diags.HasError() {
		t.Logf("Warnings getting state: %v", diags)
	}

	// Verify idempotency - NetworkId should be saved immediately after CreateNetwork succeeds
	require.False(t, stateAfterCreate.NetworkId.IsNull(), "BUG: NetworkId should be saved to state immediately after CreateNetwork API succeeds")
	require.NotEmpty(t, stateAfterCreate.NetworkId.ValueString(), "NetworkId should not be empty")

	// Verify all expected fields are set in state after failed wait
	require.Equal(t, networkId, stateAfterCreate.NetworkId.ValueString())
	require.Equal(t, fmt.Sprintf("%s,%s", projectId, networkId), stateAfterCreate.Id.ValueString())
	require.Equal(t, projectId, stateAfterCreate.ProjectId.ValueString())
	require.Equal(t, networkName, stateAfterCreate.Name.ValueString())

	_ = gateway
	_ = publicIP
	_ = prefixes
	_ = nameservers
}

func TestCreate_V1_Success(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	networkId := "network-abc-123"
	networkName := "my-test-network"
	ipv4Prefix := "10.0.0.0/24"
	gateway := "10.0.0.1"
	publicIP := "1.2.3.4"
	prefixes := []string{ipv4Prefix}
	nameservers := []string{"8.8.8.8"}

	// Setup mock expectations
	networkResp := BuildNetwork(networkId, networkName, gateway, publicIP, prefixes, nameservers)

	// Mock CreateNetwork
	mockCreateReq := mock_network_v1.NewMockApiCreateNetworkRequest(tc.MockCtrl)
	mockCreateReq.EXPECT().
		CreateNetworkPayload(gomock.Any()).
		Return(mockCreateReq).
		Times(1)
	mockCreateReq.EXPECT().
		Execute().
		Return(&iaas.Network{NetworkId: utils.Ptr(networkId)}, nil).
		Times(1)

	tc.MockClient.EXPECT().
		CreateNetwork(gomock.Any(), projectId).
		Return(mockCreateReq).
		Times(1)

	// Mock GetNetwork for wait handler
	tc.MockClient.EXPECT().
		GetNetworkExecute(gomock.Any(), projectId, networkId).
		Return(networkResp, nil).
		AnyTimes()

	// Prepare request
	schema := tc.GetSchema()
	model := CreateTestModel(projectId, "", networkName, ipv4Prefix)
	req := CreateRequest(tc.Ctx, schema, model)
	resp := CreateResponse(schema)

	tc.Resource.Create(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Create should succeed, but got errors: %v", resp.Diagnostics.Errors())

	var finalState networkModel.Model
	diags := resp.State.Get(tc.Ctx, &finalState)
	require.False(t, diags.HasError(), "Failed to get state: %v", diags.Errors())

	// Verify all fields match what was returned from GetNetwork
	require.Equal(t, networkId, finalState.NetworkId.ValueString())
	require.Equal(t, fmt.Sprintf("%s,%s", projectId, networkId), finalState.Id.ValueString())
	require.Equal(t, projectId, finalState.ProjectId.ValueString())
	require.Equal(t, networkName, finalState.Name.ValueString())
	require.Equal(t, gateway, finalState.IPv4Gateway.ValueString())
	require.Equal(t, publicIP, finalState.PublicIP.ValueString())
}

func TestCreate_V1_APICallFails(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project"
	networkName := "test-network"
	ipv4Prefix := "10.0.0.0/24"

	// Setup mock expectations
	apiErr := &oapierror.GenericOpenAPIError{}

	// Mock CreateNetwork to fail
	mockCreateReq := mock_network_v1.NewMockApiCreateNetworkRequest(tc.MockCtrl)
	mockCreateReq.EXPECT().
		CreateNetworkPayload(gomock.Any()).
		Return(mockCreateReq).
		Times(1)
	mockCreateReq.EXPECT().
		Execute().
		Return(nil, apiErr).
		Times(1)

	tc.MockClient.EXPECT().
		CreateNetwork(gomock.Any(), projectId).
		Return(mockCreateReq).
		Times(1)

	schema := tc.GetSchema()
	model := CreateTestModel(projectId, "", networkName, ipv4Prefix)
	req := CreateRequest(tc.Ctx, schema, model)
	resp := CreateResponse(schema)

	tc.Resource.Create(tc.Ctx, req, resp)

	require.True(t, resp.Diagnostics.HasError(), "Expected error when API call fails")

	// State should be empty since network was never created
	var stateAfterCreate networkModel.Model
	resp.State.Get(tc.Ctx, &stateAfterCreate)

	require.True(t, stateAfterCreate.NetworkId.IsNull(), "NetworkId should be null when API call fails")
}

func TestCreate_V2_ContextCanceledDuringWait(t *testing.T) {
	tc := NewTestContextAlpha(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	region := "eu01"
	networkId := "network-abc-123"
	networkName := "my-test-network"
	ipv4Prefix := "10.0.0.0/24"
	gateway := "10.0.0.1"
	publicIP := "1.2.3.4"
	prefixes := []string{ipv4Prefix}
	nameservers := []string{"8.8.8.8"}

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	tc.Ctx = ctx

	// Mock CreateNetwork
	mockCreateReq := mock_network_v2.NewMockApiCreateNetworkRequest(tc.MockCtrl)
	mockCreateReq.EXPECT().
		CreateNetworkPayload(gomock.Any()).
		Return(mockCreateReq).
		Times(1)
	mockCreateReq.EXPECT().
		Execute().
		Return(&iaasalpha.Network{Id: utils.Ptr(networkId)}, nil).
		Times(1)

	tc.MockClientAlpha.EXPECT().
		CreateNetwork(gomock.Any(), projectId, region).
		Return(mockCreateReq).
		Times(1)

	// Mock GetNetwork for wait handler
	tc.MockClientAlpha.EXPECT().
		GetNetworkExecute(gomock.Any(), projectId, region, networkId).
		DoAndReturn(func(ctx context.Context, projectId, region, networkId string) (*iaasalpha.Network, error) {
			time.Sleep(150 * time.Millisecond) // Longer than context timeout
			return nil, ctx.Err()
		}).
		AnyTimes()

	// Prepare request
	schema := tc.GetSchema()
	model := CreateTestModelAlpha(projectId, "", networkName, region, ipv4Prefix)
	req := CreateRequest(tc.Ctx, schema, model)
	resp := CreateResponse(schema)

	// Execute Create
	tc.Resource.Create(tc.Ctx, req, resp)

	// Assertions
	require.True(t, resp.Diagnostics.HasError(), "Expected error due to context timeout")

	var stateAfterCreate networkModel.Model
	diags := resp.State.Get(tc.Ctx, &stateAfterCreate)
	if diags.HasError() {
		t.Logf("Warnings getting state: %v", diags)
	}

	// Verify idempotency - NetworkId should be saved immediately after CreateNetwork succeeds
	require.False(t, stateAfterCreate.NetworkId.IsNull(), "BUG: NetworkId should be saved to state immediately after CreateNetwork API succeeds")
	require.NotEmpty(t, stateAfterCreate.NetworkId.ValueString(), "NetworkId should not be empty")

	// Verify all expected fields are set in state after failed wait
	require.Equal(t, networkId, stateAfterCreate.NetworkId.ValueString())
	require.Equal(t, fmt.Sprintf("%s,%s,%s", projectId, region, networkId), stateAfterCreate.Id.ValueString())
	require.Equal(t, projectId, stateAfterCreate.ProjectId.ValueString())
	require.Equal(t, networkName, stateAfterCreate.Name.ValueString())
	require.Equal(t, region, stateAfterCreate.Region.ValueString())

	_ = gateway
	_ = publicIP
	_ = prefixes
	_ = nameservers
}

func TestCreate_V2_Success(t *testing.T) {
	tc := NewTestContextAlpha(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	region := "eu01"
	networkId := "network-abc-123"
	networkName := "my-test-network"
	ipv4Prefix := "10.0.0.0/24"
	gateway := "10.0.0.1"
	publicIP := "1.2.3.4"
	prefixes := []string{ipv4Prefix}
	nameservers := []string{"8.8.8.8"}

	// Setup mock expectations
	networkResp := BuildNetworkAlpha(networkId, networkName, gateway, publicIP, prefixes, nameservers)

	// Mock CreateNetwork
	mockCreateReq := mock_network_v2.NewMockApiCreateNetworkRequest(tc.MockCtrl)
	mockCreateReq.EXPECT().
		CreateNetworkPayload(gomock.Any()).
		Return(mockCreateReq).
		Times(1)
	mockCreateReq.EXPECT().
		Execute().
		Return(&iaasalpha.Network{Id: utils.Ptr(networkId)}, nil).
		Times(1)

	tc.MockClientAlpha.EXPECT().
		CreateNetwork(gomock.Any(), projectId, region).
		Return(mockCreateReq).
		Times(1)

	// Mock GetNetwork for wait handler
	tc.MockClientAlpha.EXPECT().
		GetNetworkExecute(gomock.Any(), projectId, region, networkId).
		Return(networkResp, nil).
		AnyTimes()

	// Prepare request
	schema := tc.GetSchema()
	model := CreateTestModelAlpha(projectId, "", networkName, region, ipv4Prefix)
	req := CreateRequest(tc.Ctx, schema, model)
	resp := CreateResponse(schema)

	tc.Resource.Create(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Create should succeed, but got errors: %v", resp.Diagnostics.Errors())

	var finalState networkModel.Model
	diags := resp.State.Get(tc.Ctx, &finalState)
	require.False(t, diags.HasError(), "Failed to get state: %v", diags.Errors())

	// Verify all fields match what was returned from GetNetwork
	require.Equal(t, networkId, finalState.NetworkId.ValueString())
	require.Equal(t, fmt.Sprintf("%s,%s,%s", projectId, region, networkId), finalState.Id.ValueString())
	require.Equal(t, projectId, finalState.ProjectId.ValueString())
	require.Equal(t, networkName, finalState.Name.ValueString())
	require.Equal(t, region, finalState.Region.ValueString())
	require.Equal(t, gateway, finalState.IPv4Gateway.ValueString())
	require.Equal(t, publicIP, finalState.PublicIP.ValueString())
}
