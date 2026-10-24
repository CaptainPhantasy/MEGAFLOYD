package plan

import (
	"os"
	"path/filepath"
	"testing"
	"github.com/stretchr/testify/require"
)

func TestCanonicalPlanEditingUsesSharedPersistentStore(t *testing.T){
	dir:=t.TempDir();svc:=NewService(dir);ctxID:="canonical-session"
	first,err:=svc.Enter(ctxID);require.NoError(t,err)
	_,err=svc.Update(ctxID,"First actual plan");require.NoError(t,err)
	_,err=svc.Update(ctxID,"Revised actual plan");require.NoError(t,err)
	observed,err:=NewService(dir).Status(ctxID);require.NoError(t,err);require.Equal(t,first.ID,observed.ID);require.Equal(t,"Revised actual plan",observed.Content)
	previous,err:=os.ReadFile(filepath.Join(svc.RevisionsDir(ctxID,first.ID),"v2.md"));require.NoError(t,err);require.Equal(t,"First actual plan",string(previous))
	require.NoError(t,svc.Exit(ctxID));observed,err=NewService(dir).Status(ctxID);require.NoError(t,err);require.False(t,observed.Active)
	_,err=svc.Update(ctxID,"must not write a finished plan");require.ErrorIs(t,err,ErrNotActive)
}
