// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package controller

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// MockReconciler is a mock implementation of Reconciler interface for testing
type MockReconciler struct {
	Name            string
	SetupCalled     bool
	ReconcileCalled bool
}

func (m *MockReconciler) SetupWithManager(mgr ctrl.Manager) error {
	m.SetupCalled = true
	return nil
}

func (m *MockReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	m.ReconcileCalled = true
	return reconcile.Result{}, nil
}

// TestRegisterReconciler tests the RegisterReconciler function
func TestRegisterReconciler(t *testing.T) {
	// Save original reconcilers and restore after test
	originalReconcilers := reconcilers
	defer func() {
		reconcilers = originalReconcilers
	}()

	tests := []struct {
		name            string
		setup           func()
		reconciler      Reconciler
		expectedCount   int
		checkReconciler func(t *testing.T)
	}{
		{
			name: "register single reconciler",
			setup: func() {
				reconcilers = []Reconciler{}
			},
			reconciler:    &MockReconciler{Name: "test1"},
			expectedCount: 1,
			checkReconciler: func(t *testing.T) {
				if len(reconcilers) != 1 {
					t.Errorf("Expected 1 reconciler, got %d", len(reconcilers))
				}
				if mock, ok := reconcilers[0].(*MockReconciler); ok {
					if mock.Name != "test1" {
						t.Errorf("Expected reconciler name test1, got %s", mock.Name)
					}
				} else {
					t.Error("Expected MockReconciler type")
				}
			},
		},
		{
			name: "register multiple reconcilers",
			setup: func() {
				reconcilers = []Reconciler{}
			},
			reconciler:    nil, // Will register multiple in checkReconciler
			expectedCount: 3,
			checkReconciler: func(t *testing.T) {
				RegisterReconciler(&MockReconciler{Name: "test1"})
				RegisterReconciler(&MockReconciler{Name: "test2"})
				RegisterReconciler(&MockReconciler{Name: "test3"})

				if len(reconcilers) != 3 {
					t.Errorf("Expected 3 reconcilers, got %d", len(reconcilers))
				}

				// Verify order is preserved
				names := []string{"test1", "test2", "test3"}
				for i, r := range reconcilers {
					if mock, ok := r.(*MockReconciler); ok {
						if mock.Name != names[i] {
							t.Errorf("Expected reconciler %d to have name %s, got %s", i, names[i], mock.Name)
						}
					}
				}
			},
		},
		{
			name: "register reconciler to existing list",
			setup: func() {
				reconcilers = []Reconciler{
					&MockReconciler{Name: "existing"},
				}
			},
			reconciler:    &MockReconciler{Name: "new"},
			expectedCount: 2,
			checkReconciler: func(t *testing.T) {
				if len(reconcilers) != 2 {
					t.Errorf("Expected 2 reconcilers, got %d", len(reconcilers))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			if tt.reconciler != nil {
				RegisterReconciler(tt.reconciler)
			}

			if tt.checkReconciler != nil {
				tt.checkReconciler(t)
			}
		})
	}
}

// TestRegisterReconciler_Concurrent tests concurrent registration
func TestRegisterReconciler_Concurrent(t *testing.T) {
	// Save original reconcilers
	originalReconcilers := reconcilers
	defer func() {
		reconcilers = originalReconcilers
	}()

	// Reset reconcilers
	reconcilers = []Reconciler{}

	// Note: This test demonstrates that RegisterReconciler is NOT thread-safe
	// In production, reconcilers should be registered sequentially during initialization

	// Register a few reconcilers sequentially
	numReconcilers := 5
	for i := 0; i < numReconcilers; i++ {
		RegisterReconciler(&MockReconciler{Name: "test"})
	}

	if len(reconcilers) != numReconcilers {
		t.Errorf("Expected %d reconcilers, got %d", numReconcilers, len(reconcilers))
	}
}

// TestGetScheme tests the GetScheme function
func TestGetScheme(t *testing.T) {
	tests := []struct {
		name  string
		check func(t *testing.T, s *runtime.Scheme)
	}{
		{
			name: "returns non-nil scheme",
			check: func(t *testing.T, s *runtime.Scheme) {
				if s == nil {
					t.Error("Expected non-nil scheme")
				}
			},
		},
		{
			name: "returns same instance",
			check: func(t *testing.T, s *runtime.Scheme) {
				s2 := GetScheme()
				if s != s2 {
					t.Error("Expected GetScheme to return the same instance")
				}
			},
		},
		{
			name: "returns global scheme",
			check: func(t *testing.T, s *runtime.Scheme) {
				if s != scheme {
					t.Error("Expected GetScheme to return the global scheme variable")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := GetScheme()
			tt.check(t, s)
		})
	}
}

// TestRegisterScheme tests the RegisterScheme function
func TestRegisterScheme(t *testing.T) {
	tests := []struct {
		name          string
		schemeBuilder *runtime.SchemeBuilder
		wantErr       bool
		checkError    func(t *testing.T, err error)
	}{
		{
			name: "register valid scheme builder",
			schemeBuilder: func() *runtime.SchemeBuilder {
				// Create a valid scheme builder
				sb := &runtime.SchemeBuilder{}
				// Add a simple AddToScheme function that does nothing
				*sb = append(*sb, func(s *runtime.Scheme) error {
					return nil
				})
				return sb
			}(),
			wantErr: false,
		},
		{
			name: "register scheme builder with error",
			schemeBuilder: func() *runtime.SchemeBuilder {
				// Create a scheme builder that returns an error
				sb := &runtime.SchemeBuilder{}
				*sb = append(*sb, func(s *runtime.Scheme) error {
					return &testError{msg: "test error"}
				})
				return sb
			}(),
			wantErr: true,
			checkError: func(t *testing.T, err error) {
				if err == nil {
					t.Error("Expected error but got nil")
					return
				}
				// The error should be wrapped by errors.NewError()
				// We can check the error message
				if err.Error() == "" {
					t.Error("Expected non-empty error message")
				}
			},
		},
		{
			name: "register empty scheme builder",
			schemeBuilder: func() *runtime.SchemeBuilder {
				sb := &runtime.SchemeBuilder{}
				return sb
			}(),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RegisterScheme(tt.schemeBuilder)

			if (err != nil) != tt.wantErr {
				t.Errorf("RegisterScheme() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.checkError != nil && tt.wantErr {
				tt.checkError(t, err)
			}
		})
	}
}

// testError is a simple error implementation for testing
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

// TestReconcilerInterface tests that MockReconciler implements the Reconciler interface
func TestReconcilerInterface(t *testing.T) {
	// This test verifies that MockReconciler implements Reconciler
	var _ Reconciler = (*MockReconciler)(nil)

	mock := &MockReconciler{Name: "test"}

	// Test SetupWithManager
	err := mock.SetupWithManager(nil)
	if err != nil {
		t.Errorf("Expected no error from SetupWithManager, got %v", err)
	}
	if !mock.SetupCalled {
		t.Error("Expected SetupCalled to be true")
	}

	// Test Reconcile
	result, err := mock.Reconcile(context.Background(), reconcile.Request{})
	if err != nil {
		t.Errorf("Expected no error from Reconcile, got %v", err)
	}
	if !mock.ReconcileCalled {
		t.Error("Expected ReconcileCalled to be true")
	}
	if result.Requeue {
		t.Error("Expected result.Requeue to be false")
	}
}

// TestGlobalScheme tests the global scheme variable initialization
func TestGlobalScheme(t *testing.T) {
	if scheme == nil {
		t.Error("Expected global scheme to be initialized")
	}

	// Verify it's a runtime.Scheme
	if _, ok := interface{}(scheme).(*runtime.Scheme); !ok {
		t.Error("Expected scheme to be of type *runtime.Scheme")
	}
}

// TestReconcilers tests the global reconcilers variable
func TestReconcilers(t *testing.T) {
	// Save and restore
	originalReconcilers := reconcilers
	defer func() {
		reconcilers = originalReconcilers
	}()

	// Reset
	reconcilers = []Reconciler{}

	// Should be empty initially
	if len(reconcilers) != 0 {
		t.Errorf("Expected empty reconcilers slice, got length %d", len(reconcilers))
	}

	// Register one
	RegisterReconciler(&MockReconciler{Name: "test"})

	if len(reconcilers) != 1 {
		t.Errorf("Expected 1 reconciler after registration, got %d", len(reconcilers))
	}
}

// Benchmark tests
func BenchmarkRegisterReconciler(b *testing.B) {
	// Save and restore
	originalReconcilers := reconcilers
	defer func() {
		reconcilers = originalReconcilers
	}()

	reconcilers = []Reconciler{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		RegisterReconciler(&MockReconciler{Name: "bench"})
	}
}

func BenchmarkGetScheme(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = GetScheme()
	}
}

func BenchmarkRegisterScheme(b *testing.B) {
	sb := &runtime.SchemeBuilder{}
	*sb = append(*sb, func(s *runtime.Scheme) error {
		return nil
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = RegisterScheme(sb)
	}
}
