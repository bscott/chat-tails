package chat

import (
	"strings"
	"testing"
)

func TestClientConstants(t *testing.T) {
	// Test that constants are set to reasonable values
	if MaxMessageLength <= 0 {
		t.Errorf("MaxMessageLength should be positive, got %d", MaxMessageLength)
	}
	
	if MaxMessageLength > 10000 {
		t.Errorf("MaxMessageLength seems too large: %d", MaxMessageLength)
	}
	
	if MessageRateLimit <= 0 {
		t.Errorf("MessageRateLimit should be positive, got %d", MessageRateLimit)
	}
	
	if RateLimitWindow <= 0 {
		t.Errorf("RateLimitWindow should be positive, got %v", RateLimitWindow)
	}
}

func TestMessageValidation(t *testing.T) {
	// Test message length constants
	tests := []struct {
		name        string
		messageLen  int
		shouldBeOK  bool
	}{
		{"short message", 10, true},
		{"medium message", 500, true},
		{"max length", MaxMessageLength, true},
		{"over max", MaxMessageLength + 1, false},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := strings.Repeat("a", tt.messageLen)
			if tt.shouldBeOK && len(msg) > MaxMessageLength {
				t.Errorf("Message of length %d should be valid but exceeds MaxMessageLength", tt.messageLen)
			}
			if !tt.shouldBeOK && len(msg) <= MaxMessageLength {
				t.Errorf("Message of length %d should be invalid but is within MaxMessageLength", tt.messageLen)
			}
		})
	}
}