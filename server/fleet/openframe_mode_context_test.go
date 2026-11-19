package fleet

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsOpenframeModeContext(t *testing.T) {
	t.Run("off without the env var or the context", func(t *testing.T) {
		t.Setenv("FLEET_OPENFRAME_MODE", "")

		require.False(t, IsOpenframeModeContext(context.Background()))
	})

	t.Run("on from the context", func(t *testing.T) {
		t.Setenv("FLEET_OPENFRAME_MODE", "")

		require.True(t, IsOpenframeModeContext(NewOpenframeModeContext(context.Background())))
	})

	t.Run("on from the env var", func(t *testing.T) {
		t.Setenv("FLEET_OPENFRAME_MODE", "1")

		require.True(t, IsOpenframeModeContext(context.Background()))
	})
}
