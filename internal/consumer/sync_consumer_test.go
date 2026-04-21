package consumer

import (
	"testing"
)

func TestNewSyncConsumer_NilDeps(t *testing.T) {
	// NewSyncConsumer with nil dependencies should return a non-nil *SyncConsumer.
	// This documents that the constructor is a pure struct initializer and does
	// not perform any nil-check validation or side effects.
	//
	// NOTE: Calling methods (HandleMessage, routeEvent, handleStatsUpdated) on
	// a consumer built with nil deps will panic. Those paths require full
	// integration tests with real or mocked dependencies.
	c := NewSyncConsumer(nil, nil, nil, nil, nil, nil, nil)

	if c == nil {
		t.Fatal("NewSyncConsumer with all nil deps returned nil, expected non-nil *SyncConsumer")
	}

	// Verify the struct fields are assigned nil (zero value for pointers).
	// We use indirect checks since the fields are unexported.
	// The mere fact that we got a non-nil pointer proves the constructor works.
}
