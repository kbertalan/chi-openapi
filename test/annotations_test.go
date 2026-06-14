package openapi_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	openapi "github.com/kbertalan/chi-openapi"
)

// --- Test Handlers for annotation parsing ---
// Handler with all annotation types
// @Summary Test summary
// @Description Test description
// @Tags foo,bar
// @Accept application/xml
// @Security ApiKeyAuth
// @Security OAuth2 read,write
// @See https://docs.example.com "API guide"
// @Param id path int true "ID param"
// @Param q query string false "Query param"
// @Success 200 TestResponse "Success desc" application/json
// @Failure 400 TestErrorResponse "Bad request"
func HandlerWithAnnotations() {}

func TestParseAnnotations_AllAnnotations(t *testing.T) {
	annotation, err := openapi.ParseAnnotations("annotations_test.go", "HandlerWithAnnotations")
	if err != nil {
		t.Fatalf("ParseAnnotations error: %v", err)
	}
	if annotation == nil {
		t.Fatal("ParseAnnotations returned nil")
	}
	if annotation.Summary != "Test summary" {
		t.Errorf("expected summary, got %q", annotation.Summary)
	}
	if annotation.Description != "Test description" {
		t.Errorf("expected description, got %q", annotation.Description)
	}
	if len(annotation.Tags) != 2 || annotation.Tags[0] != "foo" || annotation.Tags[1] != "bar" {
		t.Errorf("expected tags [foo bar], got %+v", annotation.Tags)
	}
	if len(annotation.Parameters) != 2 {
		t.Errorf("expected 2 parameters, got %+v", annotation.Parameters)
	}
	// Accept
	if len(annotation.Accept) != 1 || annotation.Accept[0] != "application/xml" {
		t.Errorf("expected Accept [application/xml], got %v", annotation.Accept)
	}
	// Security: each @Security directive is one requirement, with optional scopes.
	if len(annotation.Security) != 2 {
		t.Fatalf("expected 2 security requirements, got %+v", annotation.Security)
	}
	if annotation.Security[0].Scheme != "ApiKeyAuth" || len(annotation.Security[0].Scopes) != 0 {
		t.Errorf("expected Security[0] ApiKeyAuth with no scopes, got %+v", annotation.Security[0])
	}
	if annotation.Security[1].Scheme != "OAuth2" ||
		len(annotation.Security[1].Scopes) != 2 ||
		annotation.Security[1].Scopes[0] != "read" ||
		annotation.Security[1].Scopes[1] != "write" {
		t.Errorf("expected Security[1] OAuth2 [read write], got %+v", annotation.Security[1])
	}
	// See -> external documentation.
	if annotation.See == nil ||
		annotation.See.URL != "https://docs.example.com" ||
		annotation.See.Description != "API guide" {
		t.Errorf("expected See {https://docs.example.com, API guide}, got %+v", annotation.See)
	}
	// Success carries explicit per-response produce content types.
	expectedSuccess := openapi.SuccessResponse{
		StatusCode:  200,
		DataType:    "TestResponse",
		Description: "Success desc",
		Produce:     []string{"application/json"},
	}
	if annotation.Success == nil || !reflect.DeepEqual(*annotation.Success, expectedSuccess) {
		t.Errorf("expected success %+v, got %+v", expectedSuccess, annotation.Success)
	}

	if l := len(annotation.Failures); l != 1 {
		t.Fatalf("expected one failure got %d", l)
	}

	// Failure omits @Produce, so Produce stays empty (builder applies the default).
	expectedFailure := openapi.ErrorResponse{
		StatusCode:  400,
		Type:        "TestErrorResponse",
		Description: "Bad request",
	}

	if got := annotation.Failures[0]; !reflect.DeepEqual(got, expectedFailure) {
		t.Errorf("expected failure %+v, got %+v", expectedFailure, got)
	}
}

func TestParseAnnotations_Empty(t *testing.T) {
	annotation, err := openapi.ParseAnnotations("annotations_test.go", "NonExistentHandler")
	if err != nil {
		t.Fatalf("ParseAnnotations error: %v", err)
	}
	if annotation != nil {
		t.Error("expected nil for non-existent handler")
	}
}

// Test handlers that reproduce the menu/coupon collision issue
// MenuList simulates the menu handler List function
// @Summary Get full menu
// @Description Retrieve the complete menu with all items
// @Tags menu
func MenuList() {}

// CouponList simulates the coupon handler List function
// @Summary List all coupons
// @Description Retrieve a list of all available coupons
// @Tags coupon
func CouponList() {}

// TestParseAnnotations_MenuCouponDistinct tests that menu and coupon handlers
// with the same function name "List" get their correct annotations
func TestParseAnnotations_MenuCouponDistinct(t *testing.T) {
	// Test menu handler annotations
	menuAnnotation, err := openapi.ParseAnnotations("annotations_test.go", "MenuList")
	if err != nil {
		t.Fatalf("ParseAnnotations for MenuList error: %v", err)
	}
	if menuAnnotation == nil {
		t.Fatal("ParseAnnotations for MenuList returned nil")
	}
	if menuAnnotation.Summary != "Get full menu" {
		t.Errorf("Menu handler: expected summary 'Get full menu', got %q", menuAnnotation.Summary)
	}

	// Test coupon handler annotations
	couponAnnotation, err := openapi.ParseAnnotations("annotations_test.go", "CouponList")
	if err != nil {
		t.Fatalf("ParseAnnotations for CouponList error: %v", err)
	}
	if couponAnnotation == nil {
		t.Fatal("ParseAnnotations for CouponList returned nil")
	}
	if couponAnnotation.Summary != "List all coupons" {
		t.Errorf("Coupon handler: expected summary 'List all coupons', got %q", couponAnnotation.Summary)
	}

	// Verify they are different
	if menuAnnotation.Summary == couponAnnotation.Summary {
		t.Errorf("Menu and coupon handlers should have different summaries, both got: %q", menuAnnotation.Summary)
	}

	// Test that calling them multiple times gives consistent results
	menuAnnotation2, err := openapi.ParseAnnotations("annotations_test.go", "MenuList")
	if err != nil {
		t.Fatalf("Second ParseAnnotations for MenuList error: %v", err)
	}
	if menuAnnotation2.Summary != "Get full menu" {
		t.Errorf("Second call: Menu handler summary changed to %q", menuAnnotation2.Summary)
	}

	couponAnnotation2, err := openapi.ParseAnnotations("annotations_test.go", "CouponList")
	if err != nil {
		t.Fatalf("Second ParseAnnotations for CouponList error: %v", err)
	}
	if couponAnnotation2.Summary != "List all coupons" {
		t.Errorf("Second call: Coupon handler summary changed to %q", couponAnnotation2.Summary)
	}
}

func TestParseAnnotations_WindowsStylePath(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "handler.go")
	source := `package temp

// Handler demonstrates parsing with Windows-style paths
// @Summary Temp summary
// @Description This handler is used in tests only
func Handler() {}
`
	if err := os.WriteFile(filePath, []byte(source), 0o644); err != nil {
		t.Fatalf("failed to write temp handler: %v", err)
	}
	windowsPath := strings.ReplaceAll(filePath, string(filepath.Separator), "\\")

	annotation, err := openapi.ParseAnnotations(windowsPath, "Handler")
	if err != nil {
		t.Fatalf("ParseAnnotations with Windows path error: %v", err)
	}
	if annotation == nil {
		t.Fatal("ParseAnnotations returned nil for Windows-style path")
	}
	if annotation.Summary != "Temp summary" {
		t.Errorf("expected summary 'Temp summary', got %q", annotation.Summary)
	}
}
