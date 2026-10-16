import {
  FLOYD_REGION_PROFILES,
  OPEN_PLATFORMS,
  type FloydRegion,
} from '@legacy-ai/floyd-code-oauth';

import { PRODUCT_NAME } from '#/constant/app';
import { FLOYD_CODE_GLOBAL_PLATFORM_VALUE } from '#/utils/region';

import { ChoicePickerComponent, type ChoiceOption } from './choice-picker';

const MAINLAND_CN_PLATFORM_VALUE = 'floyd-code';

const NO_PLATFORM_VALUE = 'no-configured-platform';

const REGION_PLATFORM_VALUES: Record<FloydRegion, string> = {
  'mainland-cn': MAINLAND_CN_PLATFORM_VALUE,
  global: FLOYD_CODE_GLOBAL_PLATFORM_VALUE,
};

const REGIONS: readonly FloydRegion[] = ['mainland-cn', 'global'];

const NO_PLATFORM_OPTION: ChoiceOption = {
  value: NO_PLATFORM_VALUE,
  label: 'No login platform configured',
};

const NO_PLATFORM_NOTICE =
  'Add your own provider with /provider, or declare one under [providers.*] in config.toml.';

function displayHost(url: string): string {
  return url.replace(/^[a-z][a-z0-9+.-]*:\/\//i, '').replace(/\/+$/, '');
}

function configuredRegionOptions(): ChoiceOption[] {
  const options: ChoiceOption[] = [];
  const hosts = new Set<string>();
  for (const region of REGIONS) {
    const profile = FLOYD_REGION_PROFILES[region];
    if (profile.oauthHost.length === 0) continue;
    const host = displayHost(profile.oauthHost);
    if (host.length === 0 || hosts.has(host)) continue;
    hosts.add(host);
    options.push({ value: REGION_PLATFORM_VALUES[region], label: `${PRODUCT_NAME} (${host})` });
  }
  return options;
}

function configuredOpenPlatformOptions(): ChoiceOption[] {
  return OPEN_PLATFORMS.filter((platform) => platform.baseUrl.length > 0).map((platform) => ({
    value: platform.id,
    label: platform.name,
    description: platform.baseUrl,
  }));
}

function platformOptions(): readonly ChoiceOption[] {
  return [...configuredRegionOptions(), ...configuredOpenPlatformOptions()];
}

export interface PlatformSelectorOptions {
  readonly onSelect: (platformId: string) => void;
  readonly onCancel: () => void;
}

export class PlatformSelectorComponent extends ChoicePickerComponent {
  constructor(opts: PlatformSelectorOptions) {
    const options = platformOptions();
    const unconfigured = options.length === 0;
    super({
      title: 'Select a platform',
      hint: unconfigured ? 'Enter close · Esc cancel' : undefined,
      notice: unconfigured ? NO_PLATFORM_NOTICE : undefined,
      noticeTone: unconfigured ? 'warning' : undefined,
      options: unconfigured ? [NO_PLATFORM_OPTION] : options,
      onSelect: (value) => {
        if (value === NO_PLATFORM_VALUE) {
          opts.onCancel();
          return;
        }
        opts.onSelect(value);
      },
      onCancel: opts.onCancel,
    });
  }
}
