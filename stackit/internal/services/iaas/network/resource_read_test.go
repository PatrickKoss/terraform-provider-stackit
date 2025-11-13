package network

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stackitcloud/stackit-sdk-go/core/oapierror"
	mock_network_v1 "github.com/stackitcloud/terraform-provider-stackit/stackit/internal/services/iaas/network/mock/v1"
	mock_network_v2 "github.com/stackitcloud/terraform-provider-stackit/stackit/internal/services/iaas/network/mock/v2"
	networkModel "github.com/stackitcloud/terraform-provider-stackit/stackit/internal/services/iaas/network/utils/model"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestRead_V1_Success(t *testing.T) {
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

	// Mock GetNetwork
	mockGetReq := mock_network_v1.NewMockApiGetNetworkRequest(tc.MockCtrl)
	mockGetReq.EXPECT().
		Execute().
		Return(networkResp, nil).
		Times(1)

	tc.MockClient.EXPECT().
		GetNetwork(gomock.Any(), projectId, networkId).
		Return(mockGetReq).
		Times(1)

	// Prepare request
	schema := tc.GetSchema()
	model := CreateTestModel(projectId, networkId, networkName, ipv4Prefix)
	req := ReadRequest(tc.Ctx, schema, model)
	resp := ReadResponse(schema)

	tc.Resource.Read(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Read should succeed, but got errors: %v", resp.Diagnostics.Errors())

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

func TestRead_V1_NotFound(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project"
	networkId := "network-not-found"
	networkName := "test-network"
	ipv4Prefix := "10.0.0.0/24"

	// Setup mock expectations - return 404
	apiErr := &oapierror.GenericOpenAPIError{
		StatusCode: http.StatusNotFound,
	}

	// Mock GetNetwork to return 404
	mockGetReq := mock_network_v1.NewMockApiGetNetworkRequest(tc.MockCtrl)
	mockGetReq.EXPECT().
		Execute().
		Return(nil, apiErr).
		Times(1)

	tc.MockClient.EXPECT().
		GetNetwork(gomock.Any(), projectId, networkId).
		Return(mockGetReq).
		Times(1)

	schema := tc.GetSchema()
	model := CreateTestModel(projectId, networkId, networkName, ipv4Prefix)
	req := ReadRequest(tc.Ctx, schema, model)
	resp := ReadResponse(schema)

	tc.Resource.Read(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Read should not error on 404")

	// State should be removed (null)
	var stateAfterRead networkModel.Model
	resp.State.Get(tc.Ctx, &stateAfterRead)

	require.True(t, stateAfterRead.NetworkId.IsNull(), "NetworkId should be null after resource not found")
}

func TestRead_V1_Gone(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project"
	networkId := "network-gone"
	networkName := "test-network"
	ipv4Prefix := "10.0.0.0/24"

	// Setup mock expectations - return 410
	apiErr := &oapierror.GenericOpenAPIError{
		StatusCode: http.StatusGone,
	}

	// Mock GetNetwork to return 410
	mockGetReq := mock_network_v1.NewMockApiGetNetworkRequest(tc.MockCtrl)
	mockGetReq.EXPECT().
		Execute().
		Return(nil, apiErr).
		Times(1)

	tc.MockClient.EXPECT().
		GetNetwork(gomock.Any(), projectId, networkId).
		Return(mockGetReq).
		Times(1)

	schema := tc.GetSchema()
	model := CreateTestModel(projectId, networkId, networkName, ipv4Prefix)
	req := ReadRequest(tc.Ctx, schema, model)
	resp := ReadResponse(schema)

	tc.Resource.Read(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Read should not error on 410")

	// State should be removed (null)
	var stateAfterRead networkModel.Model
	resp.State.Get(tc.Ctx, &stateAfterRead)

	require.True(t, stateAfterRead.NetworkId.IsNull(), "NetworkId should be null after resource is gone")
}

func TestRead_V2_Success(t *testing.T) {
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

	// Mock GetNetwork
	mockGetReq := mock_network_v2.NewMockApiGetNetworkRequest(tc.MockCtrl)
	mockGetReq.EXPECT().
		Execute().
		Return(networkResp, nil).
		Times(1)

	tc.MockClientAlpha.EXPECT().
		GetNetwork(gomock.Any(), projectId, region, networkId).
		Return(mockGetReq).
		Times(1)

	// Prepare request
	schema := tc.GetSchema()
	model := CreateTestModelAlpha(projectId, networkId, networkName, region, ipv4Prefix)
	req := ReadRequest(tc.Ctx, schema, model)
	resp := ReadResponse(schema)

	tc.Resource.Read(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Read should succeed, but got errors: %v", resp.Diagnostics.Errors())

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

func TestRead_V2_NotFound(t *testing.T) {
	tc := NewTestContextAlpha(t)
	defer tc.Close()

	projectId := "test-project"
	region := "eu01"
	networkId := "network-not-found"
	networkName := "test-network"
	ipv4Prefix := "10.0.0.0/24"

	// Setup mock expectations - return 404
	apiErr := &oapierror.GenericOpenAPIError{
		StatusCode: http.StatusNotFound,
	}

	// Mock GetNetwork to return 404
	mockGetReq := mock_network_v2.NewMockApiGetNetworkRequest(tc.MockCtrl)
	mockGetReq.EXPECT().
		Execute().
		Return(nil, apiErr).
		Times(1)

	tc.MockClientAlpha.EXPECT().
		GetNetwork(gomock.Any(), projectId, region, networkId).
		Return(mockGetReq).
		Times(1)

	schema := tc.GetSchema()
	model := CreateTestModelAlpha(projectId, networkId, networkName, region, ipv4Prefix)
	req := ReadRequest(tc.Ctx, schema, model)
	resp := ReadResponse(schema)

	tc.Resource.Read(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Read should not error on 404")

	// State should be removed (null)
	var stateAfterRead networkModel.Model
	resp.State.Get(tc.Ctx, &stateAfterRead)

	require.True(t, stateAfterRead.NetworkId.IsNull(), "NetworkId should be null after resource not found")
}
