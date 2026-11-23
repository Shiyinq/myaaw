package provider

import "testing"

func TestStripContextBlocksStripsInteractionEnvelope(t *testing.T) {
	raw := "<userinfo>\nChannel: api\n</userinfo>\n\n" +
		InteractionBlock(InteractionForm, map[string]any{
			"block_id": "blk_1",
			"values":   map[string]any{"name": "Alice"},
		}, "Registration: Name: Alice")

	got := StripContextBlocks(raw)
	want := "Registration: Name: Alice"
	if got != want {
		t.Fatalf("StripContextBlocks() = %q, want %q", got, want)
	}
}
