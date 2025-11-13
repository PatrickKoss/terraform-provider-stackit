package network

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stackitcloud/stackit-sdk-go/core/oapierror"
	"github.com/stackitcloud/stackit-sdk-go/core/utils"
	mock_network_v1 "github.com/stackitcloud/terraform-provider-stackit/stackit/internal/services/iaas/network/mock/v1"
	mock_network_v2 "github.com/stackitcloud/terraform-provider-stackit/stackit/internal/services/iaas/network/mock/v2"
	networkModel "github.com/stackitcloud/terraform-provider-stackit/stackit/internal/services/iaas/network/utils/model"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestUpdate_V1_Success(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	networkId := "network-abc-123"
	networkName := "my-test-network"
	updatedName := "my-updated-network"
	ipv4Prefix := "10.0.0.0/24"
	gateway := "10.0.0.1"
	publicIP := "1.2.3.4"
	prefixes := []string{ipv4Prefix}
	nameservers := []string{"8.8.8.8"}

	// Current state
	currentModel := CreateTestModel(projectId, networkId, networkName, ipv4Prefix)

	// Planned state with updated name
	plannedModel := CreateTestModel(projectId, networkId, updatedName, ipv4Prefix)

	// Setup mock expectations
	networkResp := BuildNetwork(networkId, updatedName, gateway, publicIP, prefixes, nameservers)

	// Mock PartialUpdateNetwork
	mockUpdateReq := mock_network_v1.NewMockApiPartialUpdateNetworkRequest(tc.MockCtrl)
	mockUpdateReq.EXPECT().
		PartialUpdateNetworkPayload(gomock.Any()).
		Return(mockUpdateReq).
		Times(1)
	mockUpdateReq.EXPECT().
		Execute().
		Return(nil).
		Times(1)

	tc.MockClient.EXPECT().
		PartialUpdateNetwork(gomock.Any(), projectId, networkId).
		Return(mockUpdateReq).
		Times(1)

	// Mock GetNetwork for wait handler
	tc.MockClient.EXPECT().
		GetNetworkExecute(gomock.Any(), projectId, networkId).
		Return(networkResp, nil).
		AnyTimes()

	// Prepare request
	schema := tc.GetSchema()
	req := UpdateRequest(tc.Ctx, schema, currentModel, plannedModel)
	resp := UpdateResponse(tc.Ctx, schema, &currentModel)

	tc.Resource.Update(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Update should succeed, but got errors: %v", resp.Diagnostics.Errors())

	var finalState networkModel.Model
	diags := resp.State.Get(tc.Ctx, &finalState)
	require.False(t, diags.HasError(), "Failed to get state: %v", diags.Errors())

	// Verify all fields match what was returned from GetNetwork
	require.Equal(t, networkId, finalState.NetworkId.ValueString())
	require.Equal(t, fmt.Sprintf("%s,%s", projectId, networkId), finalState.Id.ValueString())
	require.Equal(t, projectId, finalState.ProjectId.ValueString())
	require.Equal(t, updatedName, finalState.Name.ValueString())
	require.Equal(t, gateway, finalState.IPv4Gateway.ValueString())
	require.Equal(t, publicIP, finalState.PublicIP.ValueString())
}

func TestUpdate_V1_APICallFails(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project"
	networkId := "network-abc"
	networkName := "test-network"
	updatedName := "updated-network"
	ipv4Prefix := "10.0.0.0/24"

	// Current state
	currentModel := CreateTestModel(projectId, networkId, networkName, ipv4Prefix)

	// Planned state with updated name
	plannedModel := CreateTestModel(projectId, networkId, updatedName, ipv4Prefix)

	// Setup mock expectations
	apiErr := &oapierror.GenericOpenAPIError{}

	// Mock PartialUpdateNetwork to fail
	mockUpdateReq := mock_network_v1.NewMockApiPartialUpdateNetworkRequest(tc.MockCtrl)
	mockUpdateReq.EXPECT().
		PartialUpdateNetworkPayload(gomock.Any()).
		Return(mockUpdateReq).
		Times(1)
	mockUpdateReq.EXPECT().
		Execute().
		Return(apiErr).
		Times(1)

	tc.MockClient.EXPECT().
		PartialUpdateNetwork(gomock.Any(), projectId, networkId).
		Return(mockUpdateReq).
		Times(1)

	schema := tc.GetSchema()
	req := UpdateRequest(tc.Ctx, schema, currentModel, plannedModel)
	resp := UpdateResponse(tc.Ctx, schema, &currentModel)

	tc.Resource.Update(tc.Ctx, req, resp)

	require.True(t, resp.Diagnostics.HasError(), "Expected error when API call fails")

	// State should remain as current state
	var stateAfterUpdate networkModel.Model
	resp.State.Get(tc.Ctx, &stateAfterUpdate)

	require.Equal(t, networkName, stateAfterUpdate.Name.ValueString(), "State should retain original name after failed update")
}

func TestUpdate_V2_Success(t *testing.T) {
	tc := NewTestContextAlpha(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	region := "eu01"
	networkId := "network-abc-123"
	networkName := "my-test-network"
	updatedName := "my-updated-network"
	ipv4Prefix := "10.0.0.0/24"
	gateway := "10.0.0.1"
	publicIP := "1.2.3.4"
	prefixes := []string{ipv4Prefix}
	nameservers := []string{"8.8.8.8"}

	// Current state
	currentModel := CreateTestModelAlpha(projectId, networkId, networkName, region, ipv4Prefix)

	// Planned state with updated name
	plannedModel := CreateTestModelAlpha(projectId, networkId, updatedName, region, ipv4Prefix)

	// Setup mock expectations
	networkResp := BuildNetworkAlpha(networkId, updatedName, gateway, publicIP, prefixes, nameservers)

	// Mock PartialUpdateNetwork
	mockUpdateReq := mock_network_v2.NewMockApiPartialUpdateNetworkRequest(tc.MockCtrl)
	mockUpdateReq.EXPECT().
		PartialUpdateNetworkPayload(gomock.Any()).
		Return(mockUpdateReq).
		Times(1)
	mockUpdateReq.EXPECT().
		Execute().
		Return(nil).
		Times(1)

	tc.MockClientAlpha.EXPECT().
		PartialUpdateNetwork(gomock.Any(), projectId, region, networkId).
		Return(mockUpdateReq).
		Times(1)

	// Mock GetNetwork for wait handler
	tc.MockClientAlpha.EXPECT().
		GetNetworkExecute(gomock.Any(), projectId, region, networkId).
		Return(networkResp, nil).
		AnyTimes()

	// Prepare request
	schema := tc.GetSchema()
	req := UpdateRequest(tc.Ctx, schema, currentModel, plannedModel)
	resp := UpdateResponse(tc.Ctx, schema, &currentModel)

	tc.Resource.Update(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Update should succeed, but got errors: %v", resp.Diagnostics.Errors())

	var finalState networkModel.Model
	diags := resp.State.Get(tc.Ctx, &finalState)
	require.False(t, diags.HasError(), "Failed to get state: %v", diags.Errors())

	// Verify all fields match what was returned from GetNetwork
	require.Equal(t, networkId, finalState.NetworkId.ValueString())
	require.Equal(t, fmt.Sprintf("%s,%s,%s", projectId, region, networkId), finalState.Id.ValueString())
	require.Equal(t, projectId, finalState.ProjectId.ValueString())
	require.Equal(t, updatedName, finalState.Name.ValueString())
	require.Equal(t, region, finalState.Region.ValueString())
	require.Equal(t, gateway, finalState.IPv4Gateway.ValueString())
	require.Equal(t, publicIP, finalState.PublicIP.ValueString())
}

func TestUpdate_V2_WithRoutingTable(t *testing.T) {
	tc := NewTestContextAlpha(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	region := "eu01"
	networkId := "network-abc-123"
	networkName := "my-test-network"
	routingTableId := "routing-table-123"
	ipv4Prefix := "10.0.0.0/24"
	gateway := "10.0.0.1"
	publicIP := "1.2.3.4"
	prefixes := []string{ipv4Prefix}
	nameservers := []string{"8.8.8.8"}

	// Current state without routing table
	currentModel := CreateTestModelAlpha(projectId, networkId, networkName, region, ipv4Prefix)

	// Planned state with routing table
	plannedModel := CreateTestModelAlpha(projectId, networkId, networkName, region, ipv4Prefix)
	plannedModel.RoutingTableID = types.StringValue(routingTableId)

	// Setup mock expectations
	networkResp := BuildNetworkAlpha(networkId, networkName, gateway, publicIP, prefixes, nameservers)
	networkResp.RoutingTableId = utils.Ptr(routingTableId)

	// Mock PartialUpdateNetwork
	mockUpdateReq := mock_network_v2.NewMockApiPartialUpdateNetworkRequest(tc.MockCtrl)
	mockUpdateReq.EXPECT().
		PartialUpdateNetworkPayload(gomock.Any()).
		Return(mockUpdateReq).
		Times(1)
	mockUpdateReq.EXPECT().
		Execute().
		Return(nil).
		Times(1)

	tc.MockClientAlpha.EXPECT().
		PartialUpdateNetwork(gomock.Any(), projectId, region, networkId).
		Return(mockUpdateReq).
		Times(1)

	// Mock GetNetwork for wait handler
	tc.MockClientAlpha.EXPECT().
		GetNetworkExecute(gomock.Any(), projectId, region, networkId).
		Return(networkResp, nil).
		AnyTimes()

	// Prepare request
	schema := tc.GetSchema()
	req := UpdateRequest(tc.Ctx, schema, currentModel, plannedModel)
	resp := UpdateResponse(tc.Ctx, schema, &currentModel)

	tc.Resource.Update(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Update should succeed, but got errors: %v", resp.Diagnostics.Errors())

	var finalState networkModel.Model
	diags := resp.State.Get(tc.Ctx, &finalState)
	require.False(t, diags.HasError(), "Failed to get state: %v", diags.Errors())

	// Verify routing table ID is set
	require.Equal(t, routingTableId, finalState.RoutingTableID.ValueString())
}
