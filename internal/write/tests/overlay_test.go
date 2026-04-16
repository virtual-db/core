package write_test

import (
	"testing"

	"github.com/AnqorDX/vdb-core/internal/delta"
	"github.com/AnqorDX/vdb-core/internal/schema"
	. "github.com/AnqorDX/vdb-core/internal/write"
)

func TestOverlay_NoSchema_ReturnsSourceUnchanged(t *testing.T) {
	d := delta.New()
	sc := schema.NewCache()

	source := []map[string]any{
		{"id": 1, "name": "alice"},
	}

	result, err := Overlay(d, sc, "users", source)
	if err != nil {
		t.Fatalf("Overlay returned unexpected error: %v", err)
	}
	if len(result) != len(source) {
		t.Fatalf("expected len %d, got %d", len(source), len(result))
	}
	for i, row := range source {
		for k, v := range row {
			if result[i][k] != v {
				t.Errorf("row %d field %q: got %v, want %v", i, k, result[i][k], v)
			}
		}
	}
}

func TestOverlay_SchemaLoaded_EmptyDelta_ReturnsSource(t *testing.T) {
	d := delta.New()
	sc := schema.NewCache()
	sc.Load("users", []string{"id", "name"}, "id")

	source := []map[string]any{
		{"id": 1, "name": "alice"},
		{"id": 2, "name": "bob"},
	}

	result, err := Overlay(d, sc, "users", source)
	if err != nil {
		t.Fatalf("Overlay returned unexpected error: %v", err)
	}
	if len(result) != len(source) {
		t.Fatalf("expected len %d, got %d", len(source), len(result))
	}
}

func TestOverlay_Tombstone_ExcludesSourceRow(t *testing.T) {
	d := delta.New()
	sc := schema.NewCache()
	sc.Load("users", []string{"id", "name"}, "id")

	srcRow := map[string]any{"id": 1, "name": "alice"}

	if err := d.ApplyDelete("users", srcRow); err != nil {
		t.Fatalf("ApplyDelete: %v", err)
	}

	result, err := Overlay(d, sc, "users", []map[string]any{srcRow})
	if err != nil {
		t.Fatalf("Overlay returned unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result after tombstone, got %d rows", len(result))
	}
}

func TestOverlay_Update_ReplacesSourceRow(t *testing.T) {
	d := delta.New()
	sc := schema.NewCache()
	sc.Load("users", []string{"id", "name"}, "id")

	srcRow := map[string]any{"id": 1, "name": "alice"}
	updRow := map[string]any{"id": 1, "name": "alicia"}

	if err := d.ApplyUpdate("users", srcRow, updRow); err != nil {
		t.Fatalf("ApplyUpdate: %v", err)
	}

	result, err := Overlay(d, sc, "users", []map[string]any{srcRow})
	if err != nil {
		t.Fatalf("Overlay returned unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result))
	}
	if got := result[0]["name"]; got != "alicia" {
		t.Errorf("expected name %q, got %q", "alicia", got)
	}
}

func TestOverlay_Insert_AppendsNewRow(t *testing.T) {
	d := delta.New()
	sc := schema.NewCache()
	sc.Load("users", []string{"id", "name"}, "id")

	newRow := map[string]any{"id": 99, "name": "charlie"}

	if err := d.ApplyInsert("users", newRow); err != nil {
		t.Fatalf("ApplyInsert: %v", err)
	}

	result, err := Overlay(d, sc, "users", []map[string]any{})
	if err != nil {
		t.Fatalf("Overlay returned unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result))
	}
	if got := result[0]["id"]; got != 99 {
		t.Errorf("expected id 99, got %v", got)
	}
}

func TestOverlay_Insert_NotDuplicated_IfAlreadyInSource(t *testing.T) {
	d := delta.New()
	sc := schema.NewCache()
	sc.Load("users", []string{"id", "name"}, "id")

	newRow := map[string]any{"id": 99, "name": "charlie"}

	if err := d.ApplyInsert("users", newRow); err != nil {
		t.Fatalf("ApplyInsert: %v", err)
	}

	result, err := Overlay(d, sc, "users", []map[string]any{newRow})
	if err != nil {
		t.Fatalf("Overlay returned unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected exactly 1 row (no duplication), got %d", len(result))
	}
}

func TestOverlay_SourceNotMutated(t *testing.T) {
	d := delta.New()
	sc := schema.NewCache()
	sc.Load("users", []string{"id", "name"}, "id")

	srcRow := map[string]any{"id": 1, "name": "alice"}
	source := []map[string]any{srcRow}

	if err := d.ApplyDelete("users", srcRow); err != nil {
		t.Fatalf("ApplyDelete: %v", err)
	}

	_, err := Overlay(d, sc, "users", source)
	if err != nil {
		t.Fatalf("Overlay returned unexpected error: %v", err)
	}

	if len(source) != 1 {
		t.Errorf("original source slice was mutated: expected len 1, got %d", len(source))
	}
	if source[0]["id"] != 1 || source[0]["name"] != "alice" {
		t.Errorf("original source row was mutated: got %v", source[0])
	}
}
