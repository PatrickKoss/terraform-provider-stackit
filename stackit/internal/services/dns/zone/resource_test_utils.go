package dns

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stackitcloud/stackit-sdk-go/core/config"
	"github.com/stackitcloud/stackit-sdk-go/services/dns"
)

// TestContext holds common test setup
type TestContext struct {
	T          *testing.T
	Server     *httptest.Server
	Client     *dns.APIClient
	Resource   *zoneResource
	Router     *mux.Router
	Ctx        context.Context
	CancelFunc context.CancelFunc
}

// NewTestContext creates a new test context with mock server
func NewTestContext(t *testing.T) *TestContext {
	router := mux.NewRouter()
	server := httptest.NewServer(router)

	client, err := dns.NewAPIClient(
		config.WithEndpoint(server.URL),
		config.WithoutAuthentication(),
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	resource := &zoneResource{
		client: client,
	}

	return &TestContext{
		T:        t,
		Server:   server,
		Client:   client,
		Resource: resource,
		Router:   router,
		Ctx:      context.Background(),
	}
}

// Close cleans up the test context
func (tc *TestContext) Close() {
	if tc.CancelFunc != nil {
		tc.CancelFunc()
	}
	tc.Server.Close()
}

// GetSchema returns the resource schema
func (tc *TestContext) GetSchema() resource.SchemaResponse {
	schemaResp := resource.SchemaResponse{}
	tc.Resource.Schema(tc.Ctx, resource.SchemaRequest{}, &schemaResp)
	return schemaResp
}

// SetupCreateZoneHandler adds mock handler for CreateZone API
func (tc *TestContext) SetupCreateZoneHandler(zoneId string, callCounter *int) {
	tc.Router.HandleFunc("/v1/projects/{projectId}/zones", func(w http.ResponseWriter, r *http.Request) {
		if callCounter != nil {
			*callCounter++
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)

		// Return minimal JSON to avoid required field issues
		jsonResp := fmt.Sprintf(`{"zone": {"id": "%s"}}`, zoneId)
		w.Write([]byte(jsonResp))
	}).Methods("POST")
}

// SetupGetZoneHandler adds mock handler for GetZone API
func (tc *TestContext) SetupGetZoneHandler(zoneId, name, dnsName, state string) {
	tc.Router.HandleFunc("/v1/projects/{projectId}/zones/{zoneId}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Return JSON directly to avoid SDK type issues
		jsonResp := fmt.Sprintf(`{
			"zone": {
				"id": "%s",
				"name": "%s",
				"dnsName": "%s",
				"state": "%s",
				"acl": "0.0.0.0/0,::/0",
				"creationFinished": "2024-01-01T00:00:00Z",
				"creationStarted": "2024-01-01T00:00:00Z",
				"defaultTTL": 3600,
				"expireTime": 1209600,
				"negativeCache": 60,
				"primaryNameServer": "ns1.example.com",
				"refreshTime": 3600,
				"retryTime": 600,
				"serialNumber": 2024010100,
				"type": "primary",
				"updateFinished": "2024-01-01T00:00:00Z",
				"updateStarted": "2024-01-01T00:00:00Z",
				"visibility": "public"
			}
		}`, zoneId, name, dnsName, state)
		w.Write([]byte(jsonResp))
	}).Methods("GET")
}

// SetupUpdateZoneHandler adds mock handler for PartialUpdateZone API
func (tc *TestContext) SetupUpdateZoneHandler(callCounter *int) {
	tc.Router.HandleFunc("/v1/projects/{projectId}/zones/{zoneId}", func(w http.ResponseWriter, r *http.Request) {
		if callCounter != nil {
			*callCounter++
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte("{}"))
	}).Methods("PATCH")
}

// SetupDeleteZoneHandler adds mock handler for DeleteZone API
func (tc *TestContext) SetupDeleteZoneHandler(callCounter *int) {
	tc.Router.HandleFunc("/v1/projects/{projectId}/zones/{zoneId}", func(w http.ResponseWriter, r *http.Request) {
		if callCounter != nil {
			*callCounter++
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte("{}"))
	}).Methods("DELETE")
}

// SetupDeleteZoneHandlerWithStatus adds mock handler for DeleteZone API with custom status
func (tc *TestContext) SetupDeleteZoneHandlerWithStatus(statusCode int, callCounter *int) {
	tc.Router.HandleFunc("/v1/projects/{projectId}/zones/{zoneId}", func(w http.ResponseWriter, r *http.Request) {
		if callCounter != nil {
			*callCounter++
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		if statusCode >= 400 {
			w.Write([]byte(`{"message": "error"}`))
		} else {
			w.Write([]byte("{}"))
		}
	}).Methods("DELETE")
}

// SetupGetZoneHandlerWithStatus adds mock handler for GetZone API with custom status
func (tc *TestContext) SetupGetZoneHandlerWithStatus(statusCode int) {
	tc.Router.HandleFunc("/v1/projects/{projectId}/zones/{zoneId}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		if statusCode >= 400 {
			w.Write([]byte(`{"message": "error"}`))
		}
	}).Methods("GET")
}

// CreateTestModel creates a properly initialized model for testing
func CreateTestModel(projectId, zoneId, name, dnsName string) Model {
	model := Model{
		ProjectId: types.StringValue(projectId),
		Name:      types.StringValue(name),
		DnsName:   types.StringValue(dnsName),
		Primaries: types.ListNull(types.StringType),
	}
	if zoneId != "" {
		model.ZoneId = types.StringValue(zoneId)
		model.Id = types.StringValue(fmt.Sprintf("%s,%s", projectId, zoneId))
	}
	return model
}

// AssertStateFieldEquals checks a single field in the model
func AssertStateFieldEquals(t *testing.T, fieldName string, got, want types.String) {
	t.Helper()
	if !got.Equal(want) {
		t.Errorf("%s mismatch: got=%s, want=%s", fieldName, got.ValueString(), want.ValueString())
	}
}

// CreateRequest creates a test Create request
func CreateRequest(ctx context.Context, schema resource.SchemaResponse, model Model) resource.CreateRequest {
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
func UpdateRequest(ctx context.Context, schema resource.SchemaResponse, plan Model, state Model) resource.UpdateRequest {
	req := resource.UpdateRequest{}
	req.Plan = tfsdk.Plan{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	req.Plan.Set(ctx, plan)

	req.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	req.State.Set(ctx, state)
	return req
}

// UpdateResponse creates a test Update response
// Optionally initialize with current state to simulate Terraform framework behavior
func UpdateResponse(ctx context.Context, schema resource.SchemaResponse, currentState *Model) *resource.UpdateResponse {
	resp := &resource.UpdateResponse{}
	resp.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	// Initialize with current state to simulate framework behavior
	// When Update errors without calling State.Set(), this state is preserved
	if currentState != nil {
		resp.State.Set(ctx, *currentState)
	}
	return resp
}

// DeleteRequest creates a test Delete request
func DeleteRequest(ctx context.Context, schema resource.SchemaResponse, state Model) resource.DeleteRequest {
	req := resource.DeleteRequest{}
	req.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	req.State.Set(ctx, state)
	return req
}

// DeleteResponse creates a test Delete response
// Optionally initialize with current state to simulate Terraform framework behavior
func DeleteResponse(ctx context.Context, schema resource.SchemaResponse, currentState *Model) *resource.DeleteResponse {
	resp := &resource.DeleteResponse{}
	resp.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	// Initialize with current state to simulate framework behavior
	// When Delete errors without calling State.RemoveResource(), this state is preserved
	if currentState != nil {
		resp.State.Set(ctx, *currentState)
	}
	return resp
}

// ReadRequest creates a test Read request
func ReadRequest(ctx context.Context, schema resource.SchemaResponse, state Model) resource.ReadRequest {
	req := resource.ReadRequest{}
	req.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	req.State.Set(ctx, state)
	return req
}

// ReadResponse creates a test Read response
func ReadResponse(ctx context.Context, schema resource.SchemaResponse, currentState *Model) *resource.ReadResponse {
	resp := &resource.ReadResponse{}
	resp.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	if currentState != nil {
		resp.State.Set(ctx, *currentState)
	}
	return resp
}
