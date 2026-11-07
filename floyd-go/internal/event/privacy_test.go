package event

import (
	"errors"
	"testing"

	"github.com/posthog/posthog-go"
	"github.com/stretchr/testify/require"
)

type privacyClient struct {
	posthog.Client
	count int
}

func (c *privacyClient) Enqueue(posthog.Message) error { c.count++; return nil }
func TestPrivacyGateBlocksAllNewAnalytics(t *testing.T) {
	oldClient, oldID, oldFlag := client, distinctId, PrivacyDisabled()
	defer func() { client = oldClient; distinctId = oldID; SetPrivacyDisabled(oldFlag) }()
	fake := &privacyClient{}
	client = fake
	distinctId = "fixture"
	SetPrivacyDisabled(true)
	send("fixture")
	Alias("fixture-user")
	Error(errors.New("fixture"))
	require.Zero(t, fake.count)
	SetPrivacyDisabled(false)
	send("fixture")
	Alias("fixture-user")
	Error(errors.New("fixture"))
	require.Equal(t, 3, fake.count)
}
