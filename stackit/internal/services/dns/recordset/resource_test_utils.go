package dns

import (
	"context"
	"encoding/json"
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
	"github.com/stackitcloud/stackit-sdk-go/core/utils"
	"github.com/stackitcloud/stackit-sdk-go/services/dns"
)

// TestContext holds common test setup
type TestContext struct {
	T          *testing.T
	Server     *httptest.Server
	Client     *dns.APIClient
	Resource   *recordSetResource
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

	resource := &recordSetResource{
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

// RecordSetResponseData represents a mock record set response with all fields
type RecordSetResponseData struct {
	RecordSetId string
	Name        string
	Records     []string
	TTL         int64
	Type        string
	State       string
	Comment     string
}

// SetupCreateRecordSetHandler adds a mock handler for CreateRecordSet API
func (tc *TestContext) SetupCreateRecordSetHandler(recordSetId string, callCounter *int) {
	tc.Router.HandleFunc("/v1/projects/{projectId}/zones/{zoneId}/rrsets", func(w http.ResponseWriter, r *http.Request) {
		if callCounter != nil {
			*callCounter++
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)

		resp := dns.RecordSetResponse{
			Rrset: &dns.RecordSet{
				Id: utils.Ptr(recordSetId),
			},
		}
		respBytes, _ := json.Marshal(resp)
		w.Write(respBytes)
	}).Methods("POST")
}

// SetupGetRecordSetHandler adds a mock handler for GetRecordSet API
func (tc *TestContext) SetupGetRecordSetHandler(resp RecordSetResponseData) {
	tc.Router.HandleFunc("/v1/projects/{projectId}/zones/{zoneId}/rrsets/{recordSetId}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Build records array
		records := []dns.Record{}
		for _, content := range resp.Records {
			records = append(records, dns.Record{
				Content: utils.Ptr(content),
			})
		}

		// Return JSON directly to avoid SDK type complexities
		jsonResp := fmt.Sprintf(`{
			"rrset": {
				"id": "%s",
				"name": "%s",
				"type": "%s",
				"ttl": %d,
				"state": "%s",
				"comment": "%s",
				"active": true,
				"records": [`,
			resp.RecordSetId, resp.Name, resp.Type, resp.TTL, resp.State, resp.Comment)

		for i, record := range resp.Records {
			if i > 0 {
				jsonResp += ","
			}
			jsonResp += fmt.Sprintf(`{"content": "%s"}`, record)
		}

		jsonResp += `]
			}
		}`
		w.Write([]byte(jsonResp))
	}).Methods("GET")
}

// SetupUpdateRecordSetHandler adds a mock handler for PartialUpdateRecordSet API
func (tc *TestContext) SetupUpdateRecordSetHandler(callCounter *int) {
	tc.Router.HandleFunc("/v1/projects/{projectId}/zones/{zoneId}/rrsets/{recordSetId}", func(w http.ResponseWriter, r *http.Request) {
		if callCounter != nil {
			*callCounter++
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte("{}"))
	}).Methods("PATCH")
}

// SetupDeleteRecordSetHandler adds a mock handler for DeleteRecordSet API
func (tc *TestContext) SetupDeleteRecordSetHandler(callCounter *int) {
	tc.Router.HandleFunc("/v1/projects/{projectId}/zones/{zoneId}/rrsets/{recordSetId}", func(w http.ResponseWriter, r *http.Request) {
		if callCounter != nil {
			*callCounter++
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte("{}"))
	}).Methods("DELETE")
}

// SetupDeleteRecordSetHandlerWithStatus adds a mock handler for DeleteRecordSet API with custom status
func (tc *TestContext) SetupDeleteRecordSetHandlerWithStatus(statusCode int, callCounter *int) {
	tc.Router.HandleFunc("/v1/projects/{projectId}/zones/{zoneId}/rrsets/{recordSetId}", func(w http.ResponseWriter, r *http.Request) {
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

// SetupGetRecordSetHandlerWithStatus adds a mock handler for GetRecordSet API with custom status
func (tc *TestContext) SetupGetRecordSetHandlerWithStatus(statusCode int) {
	tc.Router.HandleFunc("/v1/projects/{projectId}/zones/{zoneId}/rrsets/{recordSetId}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		if statusCode >= 400 {
			w.Write([]byte(`{"message": "error"}`))
		}
	}).Methods("GET")
}

// CreateTestModel creates a properly initialized model for testing
func CreateTestModel(projectId, zoneId, recordSetId, name, recordType string, records []string) Model {
	recordsList, _ := types.ListValueFrom(context.Background(), types.StringType, records)

	model := Model{
		ProjectId: types.StringValue(projectId),
		ZoneId:    types.StringValue(zoneId),
		Name:      types.StringValue(name),
		Type:      types.StringValue(recordType),
		Records:   recordsList,
		TTL:       types.Int64Value(3600),
	}
	if recordSetId != "" {
		model.RecordSetId = types.StringValue(recordSetId)
		model.Id = types.StringValue(fmt.Sprintf("%s,%s,%s", projectId, zoneId, recordSetId))
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

// AssertStateFieldInt64Equals checks a single int64 field in the model
func AssertStateFieldInt64Equals(t *testing.T, fieldName string, got, want types.Int64) {
	t.Helper()
	if !got.Equal(want) {
		t.Errorf("%s mismatch: got=%d, want=%d", fieldName, got.ValueInt64(), want.ValueInt64())
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
