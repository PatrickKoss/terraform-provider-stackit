package mariadb

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
	"github.com/stackitcloud/stackit-sdk-go/services/mariadb"
)

// TestContext holds common test setup
type TestContext struct {
	T          *testing.T
	Server     *httptest.Server
	Client     *mariadb.APIClient
	Resource   *instanceResource
	Router     *mux.Router
	Ctx        context.Context
	CancelFunc context.CancelFunc
}

// NewTestContext creates a new test context with mock server
func NewTestContext(t *testing.T) *TestContext {
	router := mux.NewRouter()
	server := httptest.NewServer(router)

	client, err := mariadb.NewAPIClient(
		config.WithEndpoint(server.URL),
		config.WithoutAuthentication(),
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	resource := &instanceResource{
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

// SetupListOfferingsHandler adds a mock handler for ListOfferings API
func (tc *TestContext) SetupListOfferingsHandler(version, planId, planName string) {
	tc.Router.HandleFunc("/v1/projects/{projectId}/offerings", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := mariadb.ListOfferingsResponse{
			Offerings: &[]mariadb.Offering{
				{
					Version: utils.Ptr(version),
					Plans: &[]mariadb.Plan{
						{
							Id:   utils.Ptr(planId),
							Name: utils.Ptr(planName),
						},
					},
				},
			},
		}
		respBytes, _ := json.Marshal(resp)
		w.Write(respBytes)
	}).Methods("GET")
}

// SetupListOfferingsHandlerMultiplePlans adds a mock handler with multiple plans
func (tc *TestContext) SetupListOfferingsHandlerMultiplePlans(version string, plans map[string]string) {
	tc.Router.HandleFunc("/v1/projects/{projectId}/offerings", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		var planList []mariadb.Plan
		for planId, planName := range plans {
			planList = append(planList, mariadb.Plan{
				Id:   utils.Ptr(planId),
				Name: utils.Ptr(planName),
			})
		}

		resp := mariadb.ListOfferingsResponse{
			Offerings: &[]mariadb.Offering{
				{
					Version: utils.Ptr(version),
					Plans:   &planList,
				},
			},
		}
		respBytes, _ := json.Marshal(resp)
		w.Write(respBytes)
	}).Methods("GET")
}

// InstanceResponse represents a mock instance response with all fields
type InstanceResponse struct {
	InstanceId   string
	Name         string
	PlanId       string
	PlanName     string
	Version      string
	DashboardUrl string
	Status       string
}

// SetupGetInstanceHandler adds a mock handler for GetInstance API
func (tc *TestContext) SetupGetInstanceHandler(resp InstanceResponse) {
	tc.Router.HandleFunc("/v1/projects/{projectId}/instances/{instanceId}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Return JSON directly to avoid SDK type complexities
		jsonResp := fmt.Sprintf(`{
			"instanceId": "%s",
			"name": "%s",
			"planId": "%s",
			"dashboardUrl": "%s",
			"status": "%s",
			"cfGuid": "cf-guid",
			"cfSpaceGuid": "cf-space-guid",
			"cfOrganizationGuid": "cf-org-guid",
			"imageUrl": "https://image.example.com",
			"lastOperation": {"type": "update", "state": "succeeded"},
			"offeringName": "mariadb",
			"offeringVersion": "%s",
			"planName": "%s",
			"parameters": {}
		}`, resp.InstanceId, resp.Name, resp.PlanId, resp.DashboardUrl, resp.Status, resp.Version, resp.PlanName)
		w.Write([]byte(jsonResp))
	}).Methods("GET")
}

// SetupCreateInstanceHandler adds a mock handler for CreateInstance API
func (tc *TestContext) SetupCreateInstanceHandler(instanceId string, callCounter *int) {
	tc.Router.HandleFunc("/v1/projects/{projectId}/instances", func(w http.ResponseWriter, r *http.Request) {
		if callCounter != nil {
			*callCounter++
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)

		resp := mariadb.CreateInstanceResponse{
			InstanceId: utils.Ptr(instanceId),
		}
		respBytes, _ := json.Marshal(resp)
		w.Write(respBytes)
	}).Methods("POST")
}

// SetupUpdateInstanceHandler adds a mock handler for UpdateInstance API
func (tc *TestContext) SetupUpdateInstanceHandler(callCounter *int) {
	tc.Router.HandleFunc("/v1/projects/{projectId}/instances/{instanceId}", func(w http.ResponseWriter, r *http.Request) {
		if callCounter != nil {
			*callCounter++
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte("{}"))
	}).Methods("PATCH")
}

// SetupDeleteInstanceHandler adds a mock handler for DeleteInstance API
func (tc *TestContext) SetupDeleteInstanceHandler(callCounter *int) {
	tc.Router.HandleFunc("/v1/projects/{projectId}/instances/{instanceId}", func(w http.ResponseWriter, r *http.Request) {
		if callCounter != nil {
			*callCounter++
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte("{}"))
	}).Methods("DELETE")
}

// AssertModelEquals checks that the model matches expected values
func AssertModelEquals(t *testing.T, model Model, expected Model) {
	t.Helper()

	if !model.Id.Equal(expected.Id) {
		t.Errorf("Id mismatch: got=%s, want=%s", model.Id.ValueString(), expected.Id.ValueString())
	}
	if !model.InstanceId.Equal(expected.InstanceId) {
		t.Errorf("InstanceId mismatch: got=%s, want=%s", model.InstanceId.ValueString(), expected.InstanceId.ValueString())
	}
	if !model.ProjectId.Equal(expected.ProjectId) {
		t.Errorf("ProjectId mismatch: got=%s, want=%s", model.ProjectId.ValueString(), expected.ProjectId.ValueString())
	}
	if !model.Name.Equal(expected.Name) {
		t.Errorf("Name mismatch: got=%s, want=%s", model.Name.ValueString(), expected.Name.ValueString())
	}
	if !model.PlanId.Equal(expected.PlanId) {
		t.Errorf("PlanId mismatch: got=%s, want=%s", model.PlanId.ValueString(), expected.PlanId.ValueString())
	}
	if !model.PlanName.Equal(expected.PlanName) {
		t.Errorf("PlanName mismatch: got=%s, want=%s", model.PlanName.ValueString(), expected.PlanName.ValueString())
	}
	if !model.Version.Equal(expected.Version) {
		t.Errorf("Version mismatch: got=%s, want=%s", model.Version.ValueString(), expected.Version.ValueString())
	}
}

// AssertStateFieldEquals checks a single field in the model
func AssertStateFieldEquals(t *testing.T, fieldName string, got, want types.String) {
	t.Helper()
	if !got.Equal(want) {
		t.Errorf("%s mismatch: got=%s, want=%s", fieldName, got.ValueString(), want.ValueString())
	}
}

// CreateTestModel creates a test model with common values
func CreateTestModel(projectId, instanceId, name, version, planName string) Model {
	return Model{
		ProjectId:  types.StringValue(projectId),
		InstanceId: types.StringValue(instanceId),
		Name:       types.StringValue(name),
		Version:    types.StringValue(version),
		PlanName:   types.StringValue(planName),
		Parameters: types.ObjectNull(parametersTypes),
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
func UpdateRequest(ctx context.Context, schema resource.SchemaResponse, currentState, plannedState Model) resource.UpdateRequest {
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
func ReadResponse(schema resource.SchemaResponse) *resource.ReadResponse {
	resp := &resource.ReadResponse{}
	resp.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	return resp
}
