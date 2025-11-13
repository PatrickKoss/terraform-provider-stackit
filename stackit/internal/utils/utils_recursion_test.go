package utils

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/require"
)

// TestSetModelFieldsToNull_ObjectWithUnknownFields tests if the function handles nested objects
// This test should FAIL if the function doesn't handle recursion
func TestSetModelFieldsToNull_ObjectWithUnknownFields(t *testing.T) {
	ctx := context.Background()

	// Create a nested object type
	nestedObjectType := types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"field1": types.StringType,
			"field2": types.Int64Type,
		},
	}

	// Create an object with Unknown fields inside
	nestedObject, diags := types.ObjectValue(
		nestedObjectType.AttrTypes,
		map[string]attr.Value{
			"field1": types.StringUnknown(), // This is Unknown inside the object
			"field2": types.Int64Unknown(),  // This is Unknown inside the object
		},
	)
	require.False(t, diags.HasError(), "Failed to create nested object")

	// Create a model with this nested object
	type TestModel struct {
		SimpleField types.String
		NestedObj   types.Object
	}

	model := TestModel{
		SimpleField: types.StringValue("known-value"),
		NestedObj:   nestedObject, // Object itself is Known, but has Unknown fields
	}

	// Call SetModelFieldsToNull
	err := SetModelFieldsToNull(ctx, &model)
	require.NoError(t, err)

	// The object itself is Known (not Unknown), so it won't be processed
	require.False(t, model.NestedObj.IsUnknown(), "Object should not be unknown")
	require.False(t, model.NestedObj.IsNull(), "Object should not be null")

	// BUG: The fields inside the object are still Unknown!
	// SetModelFieldsToNull doesn't recursively process object fields
	attrs := model.NestedObj.Attributes()

	field1 := attrs["field1"].(types.String)
	field2 := attrs["field2"].(types.Int64)

	if field1.IsUnknown() {
		t.Errorf("BUG DETECTED: field1 inside nested object is still Unknown. SetModelFieldsToNull doesn't handle recursion for object fields.")
	}
	if field2.IsUnknown() {
		t.Errorf("BUG DETECTED: field2 inside nested object is still Unknown. SetModelFieldsToNull doesn't handle recursion for object fields.")
	}

	// This test will FAIL, demonstrating the bug
	require.False(t, field1.IsUnknown(), "Expected field1 to be null, not unknown (if recursion worked)")
	require.False(t, field2.IsUnknown(), "Expected field2 to be null, not unknown (if recursion worked)")
}

// TestSetModelFieldsToNull_ListOfObjectsWithUnknownFields tests if the function handles lists of objects
// This test should FAIL if the function doesn't handle recursion for lists
func TestSetModelFieldsToNull_ListOfObjectsWithUnknownFields(t *testing.T) {
	ctx := context.Background()

	// Create an object type for list elements
	elementObjectType := types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"name":  types.StringType,
			"value": types.Int64Type,
		},
	}

	// Create objects with Unknown fields
	obj1, diags1 := types.ObjectValue(
		elementObjectType.AttrTypes,
		map[string]attr.Value{
			"name":  types.StringValue("item1"),
			"value": types.Int64Unknown(), // Unknown inside object
		},
	)
	require.False(t, diags1.HasError())

	obj2, diags2 := types.ObjectValue(
		elementObjectType.AttrTypes,
		map[string]attr.Value{
			"name":  types.StringUnknown(), // Unknown inside object
			"value": types.Int64Value(42),
		},
	)
	require.False(t, diags2.HasError())

	// Create a list of these objects
	listOfObjects, diagsList := types.ListValue(
		elementObjectType,
		[]attr.Value{obj1, obj2},
	)
	require.False(t, diagsList.HasError())

	type TestModel struct {
		Items types.List
	}

	model := TestModel{
		Items: listOfObjects, // List is Known, but objects inside have Unknown fields
	}

	// Call SetModelFieldsToNull
	err := SetModelFieldsToNull(ctx, &model)
	require.NoError(t, err)

	// The list itself is Known (not Unknown), so it won't be processed
	require.False(t, model.Items.IsUnknown(), "List should not be unknown")
	require.False(t, model.Items.IsNull(), "List should not be null")

	// BUG: The fields inside the objects in the list are still Unknown!
	elements := model.Items.Elements()
	require.Equal(t, 2, len(elements))

	obj1Result := elements[0].(types.Object)
	obj2Result := elements[1].(types.Object)

	obj1Attrs := obj1Result.Attributes()
	obj2Attrs := obj2Result.Attributes()

	value1 := obj1Attrs["value"].(types.Int64)
	name2 := obj2Attrs["name"].(types.String)

	if value1.IsUnknown() {
		t.Errorf("BUG DETECTED: 'value' field in first list element is still Unknown. SetModelFieldsToNull doesn't handle recursion for list elements.")
	}
	if name2.IsUnknown() {
		t.Errorf("BUG DETECTED: 'name' field in second list element is still Unknown. SetModelFieldsToNull doesn't handle recursion for list elements.")
	}

	// These assertions will FAIL, demonstrating the bug
	require.False(t, value1.IsUnknown(), "Expected value to be null, not unknown (if recursion worked)")
	require.False(t, name2.IsUnknown(), "Expected name to be null, not unknown (if recursion worked)")
}

// TestSetModelFieldsToNull_CurrentBehavior_NowWithRecursion documents the NEW behavior with recursion
func TestSetModelFieldsToNull_CurrentBehavior_NowWithRecursion(t *testing.T) {
	ctx := context.Background()

	nestedObjectType := types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"field1": types.StringType,
		},
	}

	// Object with Unknown field inside
	nestedObject, _ := types.ObjectValue(
		nestedObjectType.AttrTypes,
		map[string]attr.Value{
			"field1": types.StringUnknown(),
		},
	)

	type TestModel struct {
		TopLevelUnknown types.String // This WILL be converted to null
		NestedObj       types.Object // This is Known, but fields inside will be recursively processed
	}

	model := TestModel{
		TopLevelUnknown: types.StringUnknown(),
		NestedObj:       nestedObject,
	}

	err := SetModelFieldsToNull(ctx, &model)
	require.NoError(t, err)

	// Top-level Unknown field IS converted to null
	require.True(t, model.TopLevelUnknown.IsNull(), "Top-level unknown fields are handled correctly")
	require.False(t, model.TopLevelUnknown.IsUnknown())

	// Nested object is not set to null because the object itself is Known
	require.False(t, model.NestedObj.IsUnknown())
	require.False(t, model.NestedObj.IsNull())

	// NEW BEHAVIOR: Fields inside the object are NOW converted to null with recursion
	attrs := model.NestedObj.Attributes()
	field1 := attrs["field1"].(types.String)
	require.False(t, field1.IsUnknown(), "NEW behavior: nested Unknown fields ARE converted to null with recursion")
	require.True(t, field1.IsNull(), "The field should be null, not unknown")

	t.Logf("NEW BEHAVIOR: SetModelFieldsToNull now handles nested structures recursively!")
}
