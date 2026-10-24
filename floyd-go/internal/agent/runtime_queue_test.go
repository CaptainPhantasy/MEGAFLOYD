package agent

import (
	"testing"
	"time"
	"github.com/legacy-ai/floyd/internal/agent/notify"
	"github.com/legacy-ai/floyd/internal/pubsub"
	"github.com/stretchr/testify/require"
)

func TestCanonicalQueueEditAbortAndSteerPreserveRunIDs(t *testing.T){
	env:=testEnv(t);broker:=pubsub.NewBroker[notify.RunComplete]();defer broker.Shutdown();events:=broker.Subscribe(t.Context())
	a:=NewSessionAgent(SessionAgentOptions{Sessions:env.sessions,Messages:env.messages,RunComplete:broker}).(*sessionAgent)
	const sid="runtime-queue"
	a.activeRequests.Set(sid,&activeCancel{cancel:func(){}});a.beginRuntimePrompt(SessionAgentCall{SessionID:sid,RunID:"active",Prompt:"current"})
	for _,id:=range []string{"steer","abort"}{_,err:=a.Run(t.Context(),SessionAgentCall{SessionID:sid,RunID:id,Prompt:id});require.NoError(t,err)}
	items,active:=a.runtimeQueue(sid);require.Len(t,items,2);require.Equal(t,"active",active.RunID);require.NotEmpty(t,items[0].ID);require.False(t,items[0].CreatedAt.IsZero())
	require.NoError(t,a.runtimeMutateQueue(sid,items[0].ID,"revised guidance",false));again,_:=a.runtimeQueue(sid);require.Equal(t,items[0].ID,again[0].ID)
	require.NoError(t,a.runtimeSteerQueue(sid,[]string{items[0].ID}))
	fold,dropped:=a.drainQueueForStep(sid);require.Empty(t,dropped);require.Len(t,fold,1);require.Equal(t,"steer",fold[0].RunID);require.Equal(t,"revised guidance",fold[0].Prompt)
	require.NoError(t,a.runtimeMutateQueue(sid,items[1].ID,"",true))
	select{case event:=<-events:require.Equal(t,"abort",event.Payload.RunID);require.True(t,event.Payload.Cancelled);case <-time.After(time.Second):t.Fatal("aborted queue call never completed")}
	// Canceling a separate queued call must not complete a consumed steer early.
	select{case event:=<-events:t.Fatalf("unexpected completion: %+v",event.Payload);default:}
	a.publishRunComplete(t.Context(),SessionAgentCall{SessionID:sid,RunID:"active"},notify.RunComplete{SessionID:sid,RunID:"active",MessageID:"actual-result"})
	seen:=map[string]bool{}
	for range 2 {select{case event:=<-events:seen[event.Payload.RunID]=true;require.Equal(t,"actual-result",event.Payload.MessageID);case <-time.After(time.Second):t.Fatal("steered caller never received the active run result")}}
	require.True(t,seen["steer"]);require.True(t,seen["active"])
	items,active=a.runtimeQueue(sid);require.Empty(t,items);require.Nil(t,active)
}
