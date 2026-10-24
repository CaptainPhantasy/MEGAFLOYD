import assert from 'node:assert/strict';
import test from 'node:test';
import { fetchRuntime } from '../src/meta.mjs';
import { chassisDbCandidates } from '../src/session-store.mjs';
test('desktop adopts only the native private store unless explicitly selected', async () => {
 const runtime={engine:'go',host:'unix:///own/runtime',workspace_id:'own-workspace',cwd:'/shared/project',data_dir:'/account/home/.megafloyd/data/workspaces/digest'};
 const options={port:1,workspace:runtime.cwd,attempts:1,fetchImpl:async()=>({ok:true,json:async()=>({code:0,data:runtime})})};
 const result=await fetchRuntime(options);assert.equal(result.ok,true);assert.equal(result.data.data_dir,runtime.data_dir);
 assert.equal((await fetchRuntime({...options,dataDir:'/shared/project/.megafloyd'})).ok,false);
 assert.deepEqual(chassisDbCandidates({workspace:runtime.cwd}),[]);
 assert.deepEqual(chassisDbCandidates({workspace:runtime.cwd,dataDir:runtime.data_dir}),[runtime.data_dir+'/floyd.db']);
});
