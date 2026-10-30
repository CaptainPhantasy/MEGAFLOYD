package approvalauthority

import (
	"encoding/json"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAuthorityLocalUnknownCannotAuthorizeWithoutBroker(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	server.Close()
	result, err := exchangeAuthenticated(client, request{Operation: "read", Namespace: "n", Session: "s"}, localPolicyDeferred)
	require.Error(t, err)
	require.Nil(t, result.Data)
	require.Zero(t, result.Revision)
}

func TestAuthorityRequiresBrokerPolicyProofBeforeSendingRequest(t *testing.T) {
	for _, local := range []localPolicyStatus{localPolicyDeferred, localPolicyAbsent} {
		client, server := net.Pipe()
		defer client.Close()
		go func() {
			defer server.Close()
			_ = json.NewEncoder(server).Encode(challenge{Nonce: strings.Repeat("a", 64), AccountUID: uint32(os.Getuid())})
		}()
		result, err := exchangeAuthenticated(client, request{Operation: "write", Data: []byte("not-sent")}, local)
		require.ErrorContains(t, err, "account policy")
		require.Nil(t, result.Data)
	}
}

func TestAuthorityRequiresFreshPolicyProofOnHistoryResponse(t *testing.T) {
	for _, verified := range []bool{false, true} {
		client, server := net.Pipe()
		defer client.Close()
		require.NoError(t, client.SetDeadline(time.Now().Add(time.Second)))
		done := make(chan struct{})
		go func() {
			defer close(done)
			defer server.Close()
			encoder := json.NewEncoder(server)
			_ = encoder.Encode(challenge{Nonce: strings.Repeat("b", 64), AccountPolicyVerified: true, AccountUID: uint32(os.Getuid())})
			var req request
			if json.NewDecoder(server).Decode(&req) != nil {
				return
			}
			_ = encoder.Encode(response{Revision: 1, Data: []byte("history"), AccountPolicyVerified: verified, AccountUID: uint32(os.Getuid())})
		}()
		result, err := exchangeAuthenticated(client, request{Operation: "read", Namespace: "n", Session: "s"}, localPolicyDeferred)
		if verified {
			require.NoError(t, err)
			require.Equal(t, []byte("history"), result.Data)
		} else {
			require.ErrorContains(t, err, "verified account policy")
			require.Nil(t, result.Data)
			require.Zero(t, result.Revision)
		}
		<-done
	}
}
