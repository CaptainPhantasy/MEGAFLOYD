package agent

import (
	"context"
	"testing"

	"github.com/legacy-ai/floyd/internal/megafloyd/style"
	"github.com/stretchr/testify/require"
)

func TestProtocolKernelHasNoIndependentStylePolicy(t *testing.T) {
	for _, value := range []string{"", "on", "off"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv(style.EnvVar, value)
			require.Equal(t, string(protocolKernelTmpl), protocolKernelPrompt())
			require.NotContains(t, protocolKernelPrompt(), style.Directive)
		})
	}
}

func TestPreparedHistoryDoesNotInventTodoOrUserAwareness(t *testing.T) {
	a := &sessionAgent{}
	history, attachments := a.preparePrompt(nil, false)
	require.Empty(t, history)
	require.Empty(t, attachments)
}

func TestPluginPromptDoesNotSupplyBehavioralPolicy(t *testing.T) {
	c := &coordinator{}
	content, err := c.RuntimePluginPrompt(t.Context(), "session")
	require.NoError(t, err)
	require.Empty(t, content)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = c.RuntimePluginPrompt(ctx, "session")
	require.ErrorIs(t, err, context.Canceled)
	settings := SessionRunSettings{profilePrompt: "selected profile", SystemPrompt: "user session instruction"}
	require.Equal(t, "selected profile\n\nuser session instruction", settings.promptAddition())
}
