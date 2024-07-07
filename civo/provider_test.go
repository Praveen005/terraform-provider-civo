package civo

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// TestProvider tests the provider configuration
func TestProvider(t *testing.T) {
	if err := Provider().InternalValidate(); err != nil {
		t.Fatalf("err: %s", err.Error())
	}
}

// TestProvider_impl tests the provider implementation
func TestProvider_impl(t *testing.T) {
	var _ *schema.Provider = Provider()
}

// TestToken tests the token configuration
func TestToken(t *testing.T) {

	// Create a temporary directory for test
	tempDir, err := os.MkdirTemp("", "civo-provider-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	// cleanup after the test
	defer os.RemoveAll(tempDir)

	credentialFile := filepath.Join(tempDir, "credential.json")

	credContent := `{"CIVO_TOKEN":"12345"}`
	err = os.WriteFile(credentialFile, []byte(credContent), 0600)
	if err != nil {
		t.Fatalf("Failed to write credentials file: %v", err)
	}

	const testToken = "12345"
	os.Setenv("CIVO_TOKEN", testToken)
	// cleanup after test
	defer os.Unsetenv("CIVO_TOKEN")
	rawProvider := Provider()

	raw := map[string]interface{}{
		"credential_file": credentialFile,
	}

	diags := rawProvider.Configure(context.Background(), terraform.NewResourceConfigRaw(raw))
	if diags.HasError() {
		t.Fatalf("provider configure failed: %s", diagnosticsToString(diags))
	}
}

func diagnosticsToString(diags diag.Diagnostics) string {
	diagsAsStrings := make([]string, len(diags))
	for i, diag := range diags {
		diagsAsStrings[i] = diag.Summary
	}

	return strings.Join(diagsAsStrings, "; ")
}
