import type { ProviderMediaContribution } from '#/llm/media/upload';
import { modelKey, type LlmModel } from '#/llm/model';

import { FloydFiles } from './files';
import { FLOYD_DEFAULT_BASE_URL } from './trait';

const filesByModel = new Map<string, FloydFiles>();

function resolveFiles(model: LlmModel): FloydFiles {
  const key = modelKey(model);
  let files = filesByModel.get(key);
  if (files === undefined) {
    files = new FloydFiles({
      apiKey: model.apiKey,
      baseUrl: model.baseUrl ?? FLOYD_DEFAULT_BASE_URL,
      defaultHeaders:
        model.defaultHeaders === undefined ? undefined : { ...model.defaultHeaders },
    });
    filesByModel.set(key, files);
  }
  return files;
}

export const floydMediaContribution: ProviderMediaContribution = {
  uploadVideo: (video, { model, signal }) => resolveFiles(model).uploadVideo(video, { signal }),
};
