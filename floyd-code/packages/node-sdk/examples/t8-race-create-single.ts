// One creator: meant to run twice as separate processes simultaneously.
import { createFloydHarness } from '@legacy-ai/floyd-code-sdk';

const workDir = process.argv[2]!;
const homeDir = process.argv[3]!;
const sessionId = process.argv[4]!;
const label = process.argv[5] ?? 'P';

const identity: any = { productName: 'floyd-code-cli', version: '0.0.1-test', platform: 'floyd_code_cli' };
const h = createFloydHarness({ identity, homeDir });

try {
  const s = await h.createSession({ workDir, id: sessionId, model: 'floyd-code/floyd-for-coding' });
  console.log(JSON.stringify({ label, ok: true, id: s.id, dir: s.summary?.sessionDir, pid: process.pid }));
} catch (error: any) {
  console.log(JSON.stringify({ label, ok: false, msg: String(error.message ?? error), code: error.code ?? error.cause?.code, pid: process.pid }));
} finally {
  await h.close();
  process.exit(0);
}
