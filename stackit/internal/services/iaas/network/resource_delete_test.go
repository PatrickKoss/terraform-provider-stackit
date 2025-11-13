package network

import (
	"net/http"
	"testing"

	"github.com/stackitcloud/stackit-sdk-go/core/oapierror"
	mock_network_v1 "github.com/stackitcloud/terraform-provider-stackit/stackit/internal/services/iaas/network/mock/v1"
	mock_network_v2 "github.com/stackitcloud/terraform-provider-stackit/stackit/internal/services/iaas/network/mock/v2"
	networkModel "github.com/stackitcloud/terraform-provider-stackit/stackit/internal/services/iaas/network/utils/model"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestDelete_V1_Success(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	networkId := "network-abc-123"
	networkName := "my-test-network"
	ipv4Prefix := "10.0.0.0/24"

	// Current state
	currentModel := CreateTestModel(projectId, networkId, networkName, ipv4Prefix)

	// Mock DeleteNetwork
	mockDeleteReq := mock_network_v1.NewMockApiDeleteNetworkRequest(tc.MockCtrl)
	mockDeleteReq.EXPECT().
		Execute().
		Return(nil).
		Times(1)

	tc.MockClient.EXPECT().
		DeleteNetwork(gomock.Any(), projectId, networkId).
		Return(mockDeleteReq).
		Times(1)

	// Mock GetNetwork for wait handler - should return 404 after deletion
	tc.MockClient.EXPECT().
		GetNetworkExecute(gomock.Any(), projectId, networkId).
		Return(nil, &oapierror.GenericOpenAPIError{StatusCode: http.StatusNotFound}).
		AnyTimes()

	// Prepare request
	schema := tc.GetSchema()
	req := DeleteRequest(tc.Ctx, schema, currentModel)
	resp := DeleteResponse(tc.Ctx, schema, nil)

	tc.Resource.Delete(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Delete should succeed, but got errors: %v", resp.Diagnostics.Errors())
}

func TestDelete_V1_AlreadyDeleted_404(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project"
	networkId := "network-already-deleted"
	networkName := "test-network"
	ipv4Prefix := "10.0.0.0/24"

	// Current state
	currentModel := CreateTestModel(projectId, networkId, networkName, ipv4Prefix)

	// Setup mock expectations - return 404
	apiErr := &oapierror.GenericOpenAPIError{
		StatusCode: http.StatusNotFound,
	}

	// Mock DeleteNetwork to return 404 (already deleted)
	mockDeleteReq := mock_network_v1.NewMockApiDeleteNetworkRequest(tc.MockCtrl)
	mockDeleteReq.EXPECT().
		Execute().
		Return(apiErr).
		Times(1)

	tc.MockClient.EXPECT().
		DeleteNetwork(gomock.Any(), projectId, networkId).
		Return(mockDeleteReq).
		Times(1)

	schema := tc.GetSchema()
	req := DeleteRequest(tc.Ctx, schema, currentModel)
	resp := DeleteResponse(tc.Ctx, schema, nil)

	tc.Resource.Delete(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Delete should succeed (idempotent) when resource is already deleted (404)")
}

func TestDelete_V1_AlreadyDeleted_410(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project"
	networkId := "network-gone"
	networkName := "test-network"
	ipv4Prefix := "10.0.0.0/24"

	// Current state
	currentModel := CreateTestModel(projectId, networkId, networkName, ipv4Prefix)

	// Setup mock expectations - return 410
	apiErr := &oapierror.GenericOpenAPIError{
		StatusCode: http.StatusGone,
	}

	// Mock DeleteNetwork to return 410 (resource is gone)
	mockDeleteReq := mock_network_v1.NewMockApiDeleteNetworkRequest(tc.MockCtrl)
	mockDeleteReq.EXPECT().
		Execute().
		Return(apiErr).
		Times(1)

	tc.MockClient.EXPECT().
		DeleteNetwork(gomock.Any(), projectId, networkId).
		Return(mockDeleteReq).
		Times(1)

	schema := tc.GetSchema()
	req := DeleteRequest(tc.Ctx, schema, currentModel)
	resp := DeleteResponse(tc.Ctx, schema, nil)

	tc.Resource.Delete(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Delete should succeed (idempotent) when resource is gone (410)")
}

func TestDelete_V1_APICallFails(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project"
	networkId := "network-abc"
	networkName := "test-network"
	ipv4Prefix := "10.0.0.0/24"

	// Current state
	currentModel := CreateTestModel(projectId, networkId, networkName, ipv4Prefix)

	// Setup mock expectations - return generic error (e.g., 500)
	apiErr := &oapierror.GenericOpenAPIError{
		StatusCode: http.StatusInternalServerError,
	}

	// Mock DeleteNetwork to fail
	mockDeleteReq := mock_network_v1.NewMockApiDeleteNetworkRequest(tc.MockCtrl)
	mockDeleteReq.EXPECT().
		Execute().
		Return(apiErr).
		Times(1)

	tc.MockClient.EXPECT().
		DeleteNetwork(gomock.Any(), projectId, networkId).
		Return(mockDeleteReq).
		Times(1)

	schema := tc.GetSchema()
	req := DeleteRequest(tc.Ctx, schema, currentModel)
	resp := DeleteResponse(tc.Ctx, schema, &currentModel)

	tc.Resource.Delete(tc.Ctx, req, resp)

	require.True(t, resp.Diagnostics.HasError(), "Expected error when API call fails with non-404/410 error")

	// State should remain (not removed)
	var stateAfterDelete networkModel.Model
	resp.State.Get(tc.Ctx, &stateAfterDelete)

	require.False(t, stateAfterDelete.NetworkId.IsNull(), "NetworkId should still be in state after failed delete")
}

func TestDelete_V2_Success(t *testing.T) {
	tc := NewTestContextAlpha(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	region := "eu01"
	networkId := "network-abc-123"
	networkName := "my-test-network"
	ipv4Prefix := "10.0.0.0/24"

	// Current state
	currentModel := CreateTestModelAlpha(projectId, networkId, networkName, region, ipv4Prefix)

	// Mock DeleteNetwork
	mockDeleteReq := mock_network_v2.NewMockApiDeleteNetworkRequest(tc.MockCtrl)
	mockDeleteReq.EXPECT().
		Execute().
		Return(nil).
		Times(1)

	tc.MockClientAlpha.EXPECT().
		DeleteNetwork(gomock.Any(), projectId, region, networkId).
		Return(mockDeleteReq).
		Times(1)

	// Mock GetNetwork for wait handler - should return 404 after deletion
	tc.MockClientAlpha.EXPECT().
		GetNetworkExecute(gomock.Any(), projectId, region, networkId).
		Return(nil, &oapierror.GenericOpenAPIError{StatusCode: http.StatusNotFound}).
		AnyTimes()

	// Prepare request
	schema := tc.GetSchema()
	req := DeleteRequest(tc.Ctx, schema, currentModel)
	resp := DeleteResponse(tc.Ctx, schema, nil)

	tc.Resource.Delete(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Delete should succeed, but got errors: %v", resp.Diagnostics.Errors())
}

func TestDelete_V2_AlreadyDeleted_404(t *testing.T) {
	tc := NewTestContextAlpha(t)
	defer tc.Close()

	projectId := "test-project"
	region := "eu01"
	networkId := "network-already-deleted"
	networkName := "test-network"
	ipv4Prefix := "10.0.0.0/24"

	// Current state
	currentModel := CreateTestModelAlpha(projectId, networkId, networkName, region, ipv4Prefix)

	// Setup mock expectations - return 404
	apiErr := &oapierror.GenericOpenAPIError{
		StatusCode: http.StatusNotFound,
	}

	// Mock DeleteNetwork to return 404 (already deleted)
	mockDeleteReq := mock_network_v1.NewMockApiDeleteNetworkRequest(tc.MockCtrl)
	mockDeleteReq.EXPECT().
		Execute().
		Return(apiErr).
		Times(1)

	tc.MockClientAlpha.EXPECT().
		DeleteNetwork(gomock.Any(), projectId, region, networkId).
		Return(mockDeleteReq).
		Times(1)

	schema := tc.GetSchema()
	req := DeleteRequest(tc.Ctx, schema, currentModel)
	resp := DeleteResponse(tc.Ctx, schema, nil)

	tc.Resource.Delete(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Delete should succeed (idempotent) when resource is already deleted (404)")
}

func TestDelete_V2_AlreadyDeleted_410(t *testing.T) {
	tc := NewTestContextAlpha(t)
	defer tc.Close()

	projectId := "test-project"
	region := "eu01"
	networkId := "network-gone"
	networkName := "test-network"
	ipv4Prefix := "10.0.0.0/24"

	// Current state
	currentModel := CreateTestModelAlpha(projectId, networkId, networkName, region, ipv4Prefix)

	// Setup mock expectations - return 410
	apiErr := &oapierror.GenericOpenAPIError{
		StatusCode: http.StatusGone,
	}

	// Mock DeleteNetwork to return 410 (resource is gone)
	mockDeleteReq := mock_network_v1.NewMockApiDeleteNetworkRequest(tc.MockCtrl)
	mockDeleteReq.EXPECT().
		Execute().
		Return(apiErr).
		Times(1)

	tc.MockClientAlpha.EXPECT().
		DeleteNetwork(gomock.Any(), projectId, region, networkId).
		Return(mockDeleteReq).
		Times(1)

	schema := tc.GetSchema()
	req := DeleteRequest(tc.Ctx, schema, currentModel)
	resp := DeleteResponse(tc.Ctx, schema, nil)

	tc.Resource.Delete(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Delete should succeed (idempotent) when resource is gone (410)")
}
