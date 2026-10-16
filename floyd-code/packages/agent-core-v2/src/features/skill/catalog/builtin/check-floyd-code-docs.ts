import type { SkillDefinition } from '#/features/skill/catalog/types';
import { parseSkillText } from '#/features/skill/catalog/parser';
import CHECK_FLOYD_CODE_DOCS_BODY from './check-floyd-code-docs.md?raw';

const PSEUDO_PATH = 'builtin://check-floyd-code-docs';

const parsed = parseSkillText({
  skillMdPath: '/builtin/skills/check-floyd-code-docs.md',
  skillDirName: 'check-floyd-code-docs',
  source: 'builtin',
  text: CHECK_FLOYD_CODE_DOCS_BODY,
});

export const CHECK_FLOYD_CODE_DOCS_SKILL: SkillDefinition = {
  ...parsed,
  path: PSEUDO_PATH,
  dir: PSEUDO_PATH,
  metadata: {
    ...parsed.metadata,
    type: parsed.metadata.type ?? 'inline',
  },
  productSpecific: true,
};
