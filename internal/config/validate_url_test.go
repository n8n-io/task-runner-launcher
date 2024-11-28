package config

import (
	"strings"
	"testing"
)

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		fieldName   string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid http URL",
			url:         "http://localhost:5679",
			fieldName:   "test_field",
			expectError: false,
		},
		{
			name:        "valid https URL",
			url:         "https://example.com",
			fieldName:   "test_field",
			expectError: false,
		},
		{
			name:        "scheme-less localhost",
			url:         "localhost:5679",
			fieldName:   "test_field",
			expectError: true,
			errorMsg:    "must use http:// or https:// scheme",
		},
		{
			name:        "invalid URL",
			url:         "http:// invalid url",
			fieldName:   "test_field",
			expectError: true,
			errorMsg:    "must be a valid URL",
		},
		{
			name:        "empty URL",
			url:         "",
			fieldName:   "test_field",
			expectError: true,
			errorMsg:    "must be a valid URL but is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateURL(tt.url, tt.fieldName)

			if tt.expectError && err == nil {
				t.Error("Expected error but got nil")
				return
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if tt.expectError && !strings.Contains(err.Error(), tt.errorMsg) {
				t.Errorf("Expected error containing %q, got %q", tt.errorMsg, err.Error())
			}
		})
	}
}
