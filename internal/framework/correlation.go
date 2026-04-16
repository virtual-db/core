package framework

import (
	"crypto/rand"
	"fmt"
	"io"
)

// NewCorrelationID creates a child CorrelationID from the given parent.
// If parent.Root is empty, this run becomes the root of a new causal chain.
func NewCorrelationID(parent CorrelationID) CorrelationID {
	id := NewID()
	root := parent.Root
	if root == "" {
		root = id
	}
	return CorrelationID{
		Root:   root,
		Parent: parent.ID,
		ID:     id,
	}
}

// NewID generates a cryptographically random UUID v4.
func NewID() string {
	var b [16]byte
	if _, err := io.ReadFull(rand.Reader, b[:]); err != nil {
		panic("framework: failed to generate correlation ID: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant bits
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
