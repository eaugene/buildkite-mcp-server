package trace

import (
	"context"
	"testing"
)

func TestNewProvider(t *testing.T) {
	provider, err := NewProvider(context.Background(), "test", "1.2.3")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if provider == nil {
		t.Fatal("expected non-nil TracerProvider")
	}
}
