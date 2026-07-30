package gerr

import (
	"errors"
	"testing"
)

func TestWithMethodsDoNotMutateOriginal(t *testing.T) {
	original := NewBusiness(400, "bad request")
	derived := original.WithMetadata("request", "one").WithCause(errors.New("cause")).WithType(TypeValidation)

	if original.GetMetadataValue("request") != "" || original.GetCause() != nil || original.GetType() != TypeBusiness {
		t.Fatal("With methods mutated the original error")
	}
	if derived.GetMetadataValue("request") != "one" || derived.GetCause() == nil || derived.GetType() != TypeValidation {
		t.Fatal("derived error did not contain requested changes")
	}
}

func TestGettersReturnDefensiveCopies(t *testing.T) {
	err := NewBusiness(400, "bad request").WithMetadata("key", "value")
	metadata := err.GetMetadata()
	metadata["key"] = "changed"
	if err.GetMetadataValue("key") != "value" {
		t.Fatal("GetMetadata exposed the internal map")
	}

	stack := err.GetStack()
	if len(stack) == 0 {
		t.Fatal("expected a captured stack")
	}
	stack[0] = "changed"
	if err.GetStack()[0] == "changed" {
		t.Fatal("GetStack exposed the internal slice")
	}
}
