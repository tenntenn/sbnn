package export_test

// Tests for the name the payload gives its version field. They sit in their
// own file rather than in export_test.go so that other work on the export
// package does not have to land in the same place.

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/tenntenn/sbnn/internal/export"
)

// The tool was renamed from sa to sbnn, and everything around this field -
// __SBNN_DATA__, the sbnn: storage keys, the page title - was renamed with
// it. A page that still says "saVersion" is a page whose one remaining
// mention of the old name is the field a reader would reach for to find out
// which sbnn wrote it.
func TestPayloadNamesTheVersionAfterTheToolItself(t *testing.T) {
	b, err := json.Marshal(export.Build(group(t, ""), "1.2.3", time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}
	if got := decoded["sbnnVersion"]; got != "1.2.3" {
		t.Errorf("sbnnVersion = %v, want 1.2.3", got)
	}
	if _, ok := decoded["saVersion"]; ok {
		t.Error("the payload still writes saVersion, the name the tool had before it was renamed")
	}

	// And the same in the page a reader actually receives.
	page, err := export.Render(export.Build(group(t, ""), "1.2.3", time.Now()), assets(), export.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page, `"sbnnVersion":"1.2.3"`) {
		t.Error("the rendered page does not carry sbnnVersion")
	}
	if strings.Contains(page, "saVersion") {
		t.Error("the rendered page still carries saVersion")
	}
}

// An sbnn built without a version says nothing rather than saying "".
func TestPayloadOmitsAnUnknownVersion(t *testing.T) {
	b, err := json.Marshal(export.Build(group(t, ""), "", time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"sbnnVersion", "saVersion"} {
		if _, ok := decoded[key]; ok {
			t.Errorf("%s is present although the version is unknown", key)
		}
	}
}
