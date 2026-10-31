package paranoia

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSelectPreservesWholeInteger(t *testing.T) {
	t.Parallel()
	for _, pair := range [][2]int{{-1, 0}, {math.MaxInt, math.MinInt}, {17, -23}, {0, -1}} {
		require.Equal(t, pair[0], Select(From(true), pair[0], pair[1]))
		require.Equal(t, pair[1], Select(From(false), pair[0], pair[1]))
	}
}

func TestRuntimeArithmeticAndEnvironment(t *testing.T) {
	t.Parallel()
	require.Zero(t, ConstantTimeAdd(math.MaxUint32, 1))
	require.Equal(t, uint32(42), ConstantTimeAdd(20, 22))
	require.True(t, NegativeZeroTrap())
	require.NoError(t, PoisonPill())
	require.NoError(t, RuntimeCheck())
}
