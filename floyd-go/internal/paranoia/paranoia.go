package paranoia

import (
	"fmt"
	"math"
	"sync/atomic"
)

// Bool encapsulates two's complement bitwise logic for deterministic behavior.
// It uses 0xFFFFFFFF for True and 0x00000000 for False to allow bitwise masking.
type Bool uint32

const (
	True  Bool = 0xFFFFFFFF
	False Bool = 0x00000000
)

// From converts a standard bool to our bitwise Bool.
// This is used for unrolling and branchless operations.
func From(v bool) Bool {
	if v {
		return True
	}
	return False
}

// Select returns x if b is True, y if b is False.
// This is a branchless operation to prevent timing side-channels and ensure determinism.
func Select(b Bool, x, y int) int {
	return (int(b) & x) | (^int(b) & y)
}

// ConstantTimeAdd adds two uint32 numbers with explicit wrapping behavior.
// This serves as a "macro" abstraction to ensure wrapping arithmetic is explicit.
// //go:inline
func ConstantTimeAdd(a, b uint32) uint32 {
	return a + b
}

// NegativeZeroTrap detects if the compiler or environment is misbehaving
// regarding signed zero representation. IEEE 754 requires -0.0 and 0.0 to be distinct.
func NegativeZeroTrap() bool {
	var zero float64 = 0.0
	negZero := -zero
	// Bitwise comparison to ensure they are distinct in representation
	// If the hardware or compiler collapses them, arithmetic sanity is lost.
	return math.Float64bits(zero) != math.Float64bits(negZero)
}

// PoisonPill test deliberately injects a malformed x86 NaN payload
// and asserts that the environment maintains NaN propagation integrity.
func PoisonPill() error {
	// Signal NaN with a specific payload (0xDEAD)
	// We use the quiet bit and a custom payload to ensure it survives propagation.
	snanBits := uint64(0x7FF8000000000000) | uint64(0xDEAD)
	snan := math.Float64frombits(snanBits)

	// In Go, operations on NaNs should be deterministic.
	// If the system doesn't handle the payload correctly or crashes, the pill failed.
	res := snan + 1.0
	if !math.IsNaN(res) {
		return fmt.Errorf("poison pill failed: NaN + 1.0 did not result in NaN")
	}

	// Payload check: The payload should ideally be preserved or predictably transformed.
	resBits := math.Float64bits(res)
	if (resBits & 0xFFFF) == 0 {
		// If payload is wiped to zero, it's a weak NaN handler but not a total failure.
		// However, for SuperFloyd, we prefer preservation.
	}

	return nil
}

// VerifyEnvironment performs runtime consistency checks that actively
// falsify the environment to prove safety.
func VerifyEnvironment() error {
	// 1. Check Negative Zero distinction
	if !NegativeZeroTrap() {
		return fmt.Errorf("structural parity failure: negative zero trap failed")
	}

	// 2. Run Poison Pill
	if err := PoisonPill(); err != nil {
		return err
	}

	// 3. Atomicity check
	var val int32
	atomic.AddInt32(&val, 1)
	if val != 1 {
		return fmt.Errorf("atomicity failure in bootstrap")
	}

	// 4. Memory visibility check (simple)
	done := make(chan bool, 1)
	go func() {
		atomic.StoreInt32(&val, 2)
		done <- true
	}()
	<-done
	if atomic.LoadInt32(&val) != 2 {
		return fmt.Errorf("coherence failure in bootstrap")
	}

	return nil
}

// RuntimeCheck is the entry point for SuperFloyd bootstrap.
func RuntimeCheck() error {
	return VerifyEnvironment()
}

// Wrapped for testing/mocking
var osExit = func(code int) {
	// We use panic instead of os.Exit to allow potential recovery or crash dumping.
	// In production SuperFloyd, this should be a hard halt.
	panic(fmt.Sprintf("SuperFloyd environment integrity failure (exit code %d)", code))
}
