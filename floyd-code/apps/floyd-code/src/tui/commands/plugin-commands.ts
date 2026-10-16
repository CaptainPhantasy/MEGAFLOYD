import type { PluginCommandDef } from '@legacy-ai/floyd-code-sdk';

import type { FloydSlashCommand } from './types';

export interface PluginSlashCommands {
  readonly commands: readonly FloydSlashCommand[];
  /** Maps a namespaced command name (`plugin:command`) to its markdown body. */
  readonly commandMap: ReadonlyMap<string, string>;
}

export function pluginCommandName(pluginId: string, name: string): string {
  return `${pluginId}:${name}`;
}

export function buildPluginSlashCommands(defs: readonly PluginCommandDef[]): PluginSlashCommands {
  const commandMap = new Map<string, string>();
  const commands = defs.map((def) => {
    const commandName = pluginCommandName(def.pluginId, def.name);
    commandMap.set(commandName, def.body);
    return {
      name: commandName,
      aliases: [],
      description: def.description,
    } satisfies FloydSlashCommand;
  });
  return { commands, commandMap };
}
