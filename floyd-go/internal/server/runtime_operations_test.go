package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/legacy-ai/floyd/internal/agent"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/db"
	"github.com/legacy-ai/floyd/internal/goal"
	"github.com/legacy-ai/floyd/internal/proto"
	"github.com/legacy-ai/floyd/internal/session"
	"github.com/legacy-ai/floyd/internal/shell"
	"github.com/stretchr/testify/require"
)

func TestCanonicalRuntimePlanSwarmAndTaskHTTP(t *testing.T){
	h:=newE2EHarness(t);dataDir:=t.TempDir();conn,err:=db.Connect(t.Context(),dataDir);require.NoError(t,err);t.Cleanup(func(){require.NoError(t,db.Release(dataDir))})
	h.app.Sessions=session.NewService(db.New(conn),conn);h.workspace.Cfg=config.NewTestStore(&config.Config{Options:&config.Options{DataDirectory:dataDir}})
	parent,err:=h.app.Sessions.Create(t.Context(),"canonical controls");require.NoError(t,err)
	base:=h.httpSrv.URL+"/v1/workspaces/"+h.workspace.ID
	request:=func(method,path string,body,result any){t.Helper();var payload []byte;if body!=nil{payload,err=json.Marshal(body);require.NoError(t,err)};req,err:=http.NewRequestWithContext(t.Context(),method,base+path,bytes.NewReader(payload));require.NoError(t,err);resp,err:=h.httpSrv.Client().Do(req);require.NoError(t,err);defer resp.Body.Close();require.Equal(t,200,resp.StatusCode);require.NoError(t,json.NewDecoder(resp.Body).Decode(result))}
	path:="/sessions/"+parent.ID
	var plan proto.RuntimePlan
	request("POST",path+"/plan",proto.RuntimePlanRequest{Action:"enter"},&plan);require.True(t,plan.Active)
	request("POST",path+"/plan",proto.RuntimePlanRequest{Action:"update",Content:"Actual shared Go plan"},&plan);request("GET",path+"/plan",nil,&plan);require.Equal(t,"Actual shared Go plan",plan.Content)
	request("POST",path+"/plan",proto.RuntimePlanRequest{Action:"approve"},&plan);require.False(t,plan.Active)
	var swarm agent.SwarmState;request("POST",path+"/swarm",map[string]any{"active":true,"max_parallel":2},&swarm);require.True(t,swarm.Active);require.Equal(t,2,swarm.MaxParallel);request("GET",path+"/swarm",nil,&swarm);require.True(t,swarm.Active)
	_,err=goal.Configure(t.Context(),h.app.Sessions,parent.ID,goal.Input{Objective:"Persisted goal"});require.NoError(t,err)
	var current goal.State;request("GET",path+"/goal",nil,&current);require.Equal(t,"Persisted goal",current.Objective);request("POST",path+"/goal",goal.Input{Control:"pause"},&current);require.Equal(t,"paused",current.Status)
	manager:=shell.GetBackgroundShellManager();job,err:=manager.StartForSession(t.Context(),h.workspace.Path,nil,"printf canonical-output","actual task",parent.ID,h.workspace.Path);require.NoError(t,err);t.Cleanup(func(){_ = manager.Remove(job.ID)})
	foreign,err:=manager.StartForSession(t.Context(),h.workspace.Path,nil,"printf foreign-output","other workspace",parent.ID,t.TempDir());require.NoError(t,err);t.Cleanup(func(){_ = manager.Remove(foreign.ID)})
	ctx,cancel:=context.WithTimeout(t.Context(),3*time.Second);defer cancel();require.True(t,job.WaitContext(ctx));require.True(t,foreign.WaitContext(ctx))
	var tasks struct{ Items []agent.RuntimeTask `json:"items"` };request("GET","/tasks",nil,&tasks);require.Len(t,tasks.Items,1);require.Equal(t,"bash:"+job.ID,tasks.Items[0].ID);require.Equal(t,"canonical-output",tasks.Items[0].OutputPreview);require.Equal(t,parent.ID,tasks.Items[0].SessionID)
	var task agent.RuntimeTask;request("GET","/tasks/bash:"+job.ID,nil,&task);require.Equal(t,"completed",task.Status);require.Equal(t,len("canonical-output"),task.OutputBytes)
}
