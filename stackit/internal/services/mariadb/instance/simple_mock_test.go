package mariadb

import (
	"testing"

	mock_instance "github.com/stackitcloud/terraform-provider-stackit/stackit/internal/services/mariadb/instance/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestSimpleMockSetup(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mock_instance.NewMockDefaultApi(ctrl)
	
	// Create resource with mock
	resource := &instanceResource{
		client: mockClient,
	}

	require.NotNil(t, resource.client, "Client should not be nil")
	t.Logf("Mock client type: %T", resource.client)
}
