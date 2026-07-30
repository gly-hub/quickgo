package validation

import (
	"strings"
	"testing"
)

func TestValidateRejectsNilAndNonStruct(t *testing.T) {
	for _, value := range []interface{}{nil, "config"} {
		if err := ValidateConfig(value); err == nil {
			t.Fatalf("expected %T to be rejected", value)
		}
	}
}

func TestValidateRejectsUnknownRule(t *testing.T) {
	config := struct {
		Name string `validate:"requird"`
	}{Name: "quickgo"}

	err := ValidateConfig(config)
	if err == nil || !strings.Contains(err.Error(), "unknown validation rule") {
		t.Fatalf("expected unknown rule error, got %v", err)
	}
}

func TestURLRuleRequiresAbsoluteURL(t *testing.T) {
	config := struct {
		Endpoint string `validate:"url"`
	}{Endpoint: "/relative"}

	if err := ValidateConfig(config); err == nil {
		t.Fatal("expected relative URL to be rejected")
	}
}
