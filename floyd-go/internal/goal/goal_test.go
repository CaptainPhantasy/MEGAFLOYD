package goal

import (
	"context"
	"testing"
	"time"
	"github.com/legacy-ai/floyd/internal/db"
	"github.com/legacy-ai/floyd/internal/session"
	"github.com/stretchr/testify/require"
)

func goalTestSession(t *testing.T)(session.Service,string){t.Helper();dir:=t.TempDir();conn,err:=db.Connect(t.Context(),dir);require.NoError(t,err);t.Cleanup(func(){require.NoError(t,db.Release(dir))});svc:=session.NewService(db.New(conn),conn);s,err:=svc.Create(t.Context(),"goal controls");require.NoError(t,err);return svc,s.ID}

func TestCanonicalGoalBudgetsAndLifecycle(t *testing.T){
	svc,id:=goalTestSession(t);ctx:=t.Context()
	s,err:=Configure(ctx,svc,id,Input{Objective:"Verify the canonical implementation"});require.NoError(t,err);require.EqualValues(t,3,*s.Budget.TurnBudget)
	for turn:=int64(1);turn<=3;turn++{
		run,err:=Begin(ctx,svc,id);require.NoError(t,err);require.NotNil(t,run);require.Contains(t,run.Prompt(),s.Objective)
		require.NoError(t,run.AddUsage(ctx,7));require.False(t,run.ShouldStop(),"turn limit must let the accepted turn finish")
		require.NoError(t,run.Finish(ctx,nil));require.NoError(t,run.Finish(ctx,nil))
		got,err:=Get(ctx,svc,id);require.NoError(t,err);require.Equal(t,turn,got.TurnsUsed);require.Equal(t,turn*7,got.TokensUsed)
		if turn<3{require.NotEmpty(t,run.Continuation(ctx))}else{require.Empty(t,run.Continuation(ctx));require.Equal(t,"blocked",got.Status);require.True(t,got.Budget.TurnBudgetReached)}
	}
	_,err=Configure(ctx,svc,id,Input{Control:"resume"});require.ErrorContains(t,err,"budget")
	limit:=int64(5);s,err=Configure(ctx,svc,id,Input{Control:"resume",Budget:&BudgetInput{TurnBudget:&limit}});require.NoError(t,err);require.Equal(t,"active",s.Status)
	run,err:=Begin(ctx,svc,id);require.NoError(t,err);_,err=Configure(ctx,svc,id,Input{Control:"pause"});require.NoError(t,err);require.True(t,run.ShouldStop());require.NoError(t,run.Finish(ctx,context.Canceled));require.Empty(t,run.Continuation(ctx))
	_,err=Configure(ctx,svc,id,Input{Control:"resume"});require.NoError(t,err);_,err=Configure(ctx,svc,id,Input{Control:"complete",Reason:"All direct checks passed"});require.NoError(t,err)
	run,err=Begin(ctx,svc,id);require.NoError(t,err);require.Nil(t,run)
}

func TestCanonicalGoalTokenWallAndReplacementIsolation(t *testing.T){
	svc,id:=goalTestSession(t);ctx:=t.Context();tokens:=int64(10);wall:=int64(1)
	_,err:=Configure(ctx,svc,id,Input{Objective:"Bounded work",Budget:&BudgetInput{TokenBudget:&tokens}});require.NoError(t,err)
	run,err:=Begin(ctx,svc,id);require.NoError(t,err);require.NoError(t,run.AddUsage(ctx,10));require.True(t,run.ShouldStop());require.NoError(t,run.Finish(ctx,nil));require.Empty(t,run.Continuation(ctx))
	_,err=Configure(ctx,svc,id,Input{Objective:"New work",Budget:&BudgetInput{WallClockBudgetMs:&wall}});require.NoError(t,err)
	run,err=Begin(ctx,svc,id);require.NoError(t,err);run.started=time.Now().Add(-time.Second)
	_,err=svc.(session.MetadataMutator).MutateMetadata(ctx,id,func(meta map[string]any)error{s,e:=read(meta);if e!=nil{return e};started:=time.Now().Add(-time.Second);s.StartedAt=&started;return store(meta,s)});require.NoError(t,err);require.True(t,run.ShouldStop());require.NoError(t,run.Finish(ctx,nil))
	state,err:=Get(ctx,svc,id);require.NoError(t,err);require.True(t,state.Budget.WallClockBudgetReached);require.GreaterOrEqual(t,state.WallClockMs,int64(1000))
}
