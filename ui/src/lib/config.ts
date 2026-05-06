export type AgoraRuntimeConfig = Readonly<{
  apiBasePath: string;
  version: string;
  demoFlags: Readonly<Record<string, boolean>>;
}>;

type RuntimeConfigInput = Partial<{
  apiBasePath: string;
  version: string;
  demoFlags: Record<string, boolean>;
}>;

declare global {
  interface Window {
    __AGORA_CONFIG__?: RuntimeConfigInput;
  }
}

const defaultConfig: AgoraRuntimeConfig = {
  apiBasePath: '/v1',
  version: 'dev',
  demoFlags: {},
};

function readRuntimeConfig(): RuntimeConfigInput {
  if (typeof window === 'undefined') {
    return {};
  }

  return window.__AGORA_CONFIG__ ?? {};
}

function normalizeConfig(config: RuntimeConfigInput): AgoraRuntimeConfig {
  return {
    apiBasePath: config.apiBasePath?.trim() || defaultConfig.apiBasePath,
    version: config.version?.trim() || defaultConfig.version,
    demoFlags: config.demoFlags ?? defaultConfig.demoFlags,
  };
}

export const agoraConfig = normalizeConfig(readRuntimeConfig());

export function getAgoraConfig(): AgoraRuntimeConfig {
  return agoraConfig;
}
