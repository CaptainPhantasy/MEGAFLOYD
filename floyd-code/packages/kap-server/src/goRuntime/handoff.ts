import { GoRequestError, type GoClient } from './client';
import { array, object, text } from './projection';

export const handoffTargets = ['web', 'tui', 'desktop'] as const;
export type HandoffTarget = typeof handoffTargets[number];
export interface HandoffDescriptor { cwd: string; data_dir: string; session_id: string; host: string; web_url: string; desktop_url: string; terminal_command: string; target?: HandoffTarget; opened?: boolean }

export function isHandoffTarget(value: unknown): value is HandoffTarget { return typeof value === 'string' && handoffTargets.includes(value as HandoffTarget); }

export function requireHandoffTarget(value: unknown): HandoffTarget {
  if (!isHandoffTarget(value)) throw new GoRequestError(400, 'Handoff target must be web, tui, or desktop');
  return value;
}

export function fixedHandoff(submission: Record<string, unknown>): HandoffTarget | undefined {
  const content = array(submission['content']);
  const prompt = content.map((part) => text(object(part)['text'])).join('\n').trim();
  const match = /^\/(?:skill:)?(web|tui|desktop)(?=\s|$)/.exec(prompt);
  const skills = array(submission['skills']).map(object);
  const selected = skills.find((skill) => isHandoffTarget(skill['name']));
  if (!match && !selected) return undefined;
  const target = requireHandoffTarget(match?.[1] ?? selected?.['name']);
  const onlyText = content.every((part) => object(part)['type'] === 'text');
  const validPrompt = match ? prompt === `/${target}` || prompt === `/skill:${target}` : prompt.length === 0;
  const validSkills = match ? skills.length === 0 : skills.length === 1 && !text(selected?.['args']).trim();
  if (!onlyText || !validPrompt || !validSkills || array(submission['attachments']).length) throw new GoRequestError(400, `Use /${target} by itself without arguments or attachments`);
  return target;
}

export function handoffCommands() {
  const names: Record<HandoffTarget, string> = { web: 'browser', tui: 'terminal', desktop: 'MegaFloyd desktop' };
  return handoffTargets.map((target) => ({ name: target, description: `Continue this same session in the ${names[target]} on this computer`, path: `megafloyd:handoff:${target}`, source: 'builtin' as const, type: 'command', disable_model_invocation: true, scopes: ['web'] }));
}

export function nativeHandoff(client: GoClient, base: string, sessionId: string, target?: HandoffTarget): Promise<HandoffDescriptor> {
  return client.json(target ? 'POST' : 'GET', sessionId ? `${base}/sessions/${encodeURIComponent(sessionId)}/handoff` : `${base}/handoff`, target ? { target } : undefined);
}
