package framework_test

import (
	"testing"

	. "github.com/AnqorDX/vdb-core/internal/framework"
)

func TestNewCorrelationID_ZeroParent_BecomesRoot(t *testing.T) {
	t.Parallel()

	cid := NewCorrelationID(CorrelationID{})

	if cid.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if cid.Root != cid.ID {
		t.Fatalf("expected Root == ID for a zero parent, got Root=%q ID=%q", cid.Root, cid.ID)
	}
	if cid.Parent != "" {
		t.Fatalf("expected Parent to be empty for a zero parent, got %q", cid.Parent)
	}
}

func TestNewCorrelationID_ExistingParent_PreservesRoot(t *testing.T) {
	t.Parallel()

	root := NewCorrelationID(CorrelationID{})

	child := NewCorrelationID(root)

	if child.Root != root.Root {
		t.Fatalf("expected child to preserve parent root %q, got %q", root.Root, child.Root)
	}
	if child.Parent != root.ID {
		t.Fatalf("expected child.Parent == root.ID (%q), got %q", root.ID, child.Parent)
	}
	if child.ID == root.ID {
		t.Fatal("child ID should differ from parent ID")
	}
	if child.ID == "" {
		t.Fatal("child ID must not be empty")
	}

	grandchild := NewCorrelationID(child)

	if grandchild.Root != root.Root {
		t.Fatalf("expected grandchild to preserve original root %q, got %q", root.Root, grandchild.Root)
	}
	if grandchild.Parent != child.ID {
		t.Fatalf("expected grandchild.Parent == child.ID (%q), got %q", child.ID, grandchild.Parent)
	}
}

func TestNewID_NonEmpty(t *testing.T) {
	t.Parallel()

	id := NewID()
	if id == "" {
		t.Fatal("NewID() returned an empty string")
	}
}

func TestNewID_Unique(t *testing.T) {
	t.Parallel()

	const iterations = 1000
	seen := make(map[string]struct{}, iterations)
	for i := 0; i < iterations; i++ {
		id := NewID()
		if _, exists := seen[id]; exists {
			t.Fatalf("NewID() returned duplicate value %q after %d iterations", id, i)
		}
		seen[id] = struct{}{}
	}
}
