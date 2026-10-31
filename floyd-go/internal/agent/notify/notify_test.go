package notify_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/legacy-ai/floyd/internal/agent/notify"
	"github.com/legacy-ai/floyd/internal/pubsub"
	"github.com/stretchr/testify/require"
)

func TestRunCompleteEventPreservesCorrelationAndFailureState(t *testing.T) {
	t.Parallel()
	broker := pubsub.NewBroker[notify.RunComplete]()
	defer broker.Shutdown()
	subscription := broker.Subscribe(t.Context())
	broker.PublishMustDeliver(t.Context(), pubsub.CreatedEvent, notify.RunComplete{SessionID: "session-a", RunID: "request-b", MessageID: "message-c", Text: "partial text", Error: "interrupted", Cancelled: true})
	select {
	case event := <-subscription:
		encoded, err := json.Marshal(event)
		require.NoError(t, err)
		require.JSONEq(t, `{"type":"created","payload":{"SessionID":"session-a","RunID":"request-b","MessageID":"message-c","Text":"partial text","Error":"interrupted","Cancelled":true}}`, string(encoded))
	case <-time.After(time.Second):
		t.Fatal("completion event did not reach its observer")
	}
}

func TestNotificationWirePayloadRetainsAuthenticationContext(t *testing.T) {
	t.Parallel()
	var event notify.Notification
	require.NoError(t, json.Unmarshal([]byte(`{"SessionID":"s","Type":"aws_sso_auth","ProviderID":"bedrock","RunID":"r","AWSSOCommand":"refresh-sso","AWSSOURL":"https://example.test/verify"}`), &event))
	require.Equal(t, notify.TypeAWSSSOAuth, event.Type)
	require.Equal(t, "bedrock", event.ProviderID)
	require.Equal(t, "r", event.RunID)
	require.Equal(t, "refresh-sso", event.AWSSOCommand)
	require.Equal(t, "https://example.test/verify", event.AWSSOURL)
}
