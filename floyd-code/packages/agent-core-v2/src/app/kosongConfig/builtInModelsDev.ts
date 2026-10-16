declare const __FLOYD_CODE_BUILT_IN_CATALOG__: string | undefined;

export const BUILT_IN_MODELS_DEV_JSON: string | undefined =
  typeof __FLOYD_CODE_BUILT_IN_CATALOG__ === 'string'
    ? __FLOYD_CODE_BUILT_IN_CATALOG__
    : undefined;
