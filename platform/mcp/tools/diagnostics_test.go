package tools

import "testing"

func TestCaptureProfileIsNotIdempotentOrReadOnly(t *testing.T) {
	definition := Diagnostics{}.CaptureProfileDefinition()
	if definition.Annotations.ReadOnlyHint == nil || *definition.Annotations.ReadOnlyHint {
		t.Fatalf("readOnlyHint = %#v, want false", definition.Annotations.ReadOnlyHint)
	}
	if definition.Annotations.IdempotentHint == nil || *definition.Annotations.IdempotentHint {
		t.Fatalf("idempotentHint = %#v, want false", definition.Annotations.IdempotentHint)
	}
}
