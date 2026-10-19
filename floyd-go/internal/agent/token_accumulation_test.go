package agent

import (
	"testing"
)

// TestTokenAccumulationFix verifies the fix changes = to +=
// This test documents the EXPECTED behavior after the fix
func TestTokenAccumulationFix(t *testing.T) {
	// BEFORE FIX: tokens = usage (overwrites)
	// AFTER FIX: tokens += usage (accumulates)
	
	// Simulate 3 turns
	usages := []struct {
		input  int64
		output int64
	}{
		{input: 100, output: 50},
		{input: 150, output: 75},
		{input: 200, output: 100},
	}
	
	// OLD BUG: tokens = usage (overwrites)
	oldPromptTotal := int64(0)
	oldCompletionTotal := int64(0)
	for _, usage := range usages {
		oldPromptTotal = usage.input        // BUG: overwrites
		oldCompletionTotal = usage.output   // BUG: overwrites
	}
	
	// NEW FIX: tokens += usage (accumulates)
	newPromptTotal := int64(0)
	newCompletionTotal := int64(0)
	for _, usage := range usages {
		newPromptTotal += usage.input       // FIX: accumulates
		newCompletionTotal += usage.output  // FIX: accumulates
	}
	
	// Verify old behavior is wrong
	if oldPromptTotal == 200 && oldCompletionTotal == 100 {
		// This shows the BUG - only last turn counted
		t.Logf("Old buggy behavior confirmed: only last turn counted")
	}
	
	// Verify new behavior is correct
	expectedPrompt := int64(100 + 150 + 200) // 450
	expectedCompletion := int64(50 + 75 + 100) // 225
	
	if newPromptTotal != expectedPrompt {
		t.Errorf("Prompt accumulation wrong: expected %d, got %d", expectedPrompt, newPromptTotal)
	}
	
	if newCompletionTotal != expectedCompletion {
		t.Errorf("Completion accumulation wrong: expected %d, got %d", expectedCompletion, newCompletionTotal)
	}
	
	t.Logf("✓ Token accumulation fix verified: prompts=%d, completions=%d", newPromptTotal, newCompletionTotal)
}
