package httpui

import (
	"bytes"
	"context"
	"gopress/internal/ui/components/select"
	"strings"
	"testing"
)

func TestAdminSelectFormContract(t *testing.T) {
	var out bytes.Buffer
	err := AdminSelect("choice", "depth", "1", "Положение", []selectcomp.ItemData{{Value: "0", Label: "Основной пункт"}, {Value: "1", Label: `<script>unsafe</script>`}}, nil).Render(context.Background(), &out)
	if err != nil {
		t.Fatal(err)
	}
	body := out.String()
	for _, marker := range []string{`role="combobox"`, `role="listbox"`, `aria-selected="true"`, `type="hidden"`, `name="depth"`, `value="1"`, `&lt;script&gt;unsafe&lt;/script&gt;`} {
		if !strings.Contains(body, marker) {
			t.Fatalf("missing %s", marker)
		}
	}
	if strings.Contains(body, "<select") || strings.Contains(body, "style=") || strings.Contains(body, "<script>") || strings.Count(body, `name="depth"`) != 1 {
		t.Fatal("native control, unsafe markup or duplicate field")
	}
}
