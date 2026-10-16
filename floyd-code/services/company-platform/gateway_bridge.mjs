import {readFileSync} from 'node:fs';
const root=process.env.FLOYD_COMPANY_GATEWAY_ROOT || '/srv/organized/services/MCP_SERVER_ROOM/mcp-gateway';
const {Client}=await import(root+'/node_modules/@modelcontextprotocol/sdk/dist/esm/client/index.js');
const {StdioClientTransport}=await import(root+'/node_modules/@modelcontextprotocol/sdk/dist/esm/client/stdio.js');
const input=JSON.parse(readFileSync(0,'utf8'));
const client=new Client({name:'floyd-company-platform',version:'1'});
let result;
try {
 await client.connect(new StdioClientTransport({command:process.execPath,args:[root+'/dist/index.js'],stderr:'pipe'}));
 const answer=await client.callTool({name:input.method,arguments:input.arguments},undefined,{timeout:45000});
 if(input.method==='call_tool') {
  let envelope=answer.structuredContent;
  if(!envelope) { try {envelope=JSON.parse(answer.content[0]?.text);} catch {} }
  result={ok:!answer.isError && envelope?.ok!==false,code:envelope?.code,data:answer};
 } else {
  const envelope=JSON.parse(answer.content[0]?.text || '{}');
  result={ok:envelope.ok===true,code:envelope.code,data:envelope.data};
 }
} catch {result={ok:false,code:'GATEWAY_CONNECTION_FAILED'};}
finally {await client.close();}
const raw=JSON.stringify(result);
process.stdout.write(raw.length <= 8*1024*1024 ? raw : JSON.stringify({ok:false,code:'RESULT_TOO_LARGE'}));
