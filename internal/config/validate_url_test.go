package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
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

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
