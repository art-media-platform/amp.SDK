import { useCallback, useState } from 'react';
import { useAmpContext } from '../provider.js';
import type { AmpUploadResult, BlobRef, UploadOpts } from '../types.js';

export function useAmpUpload(): AmpUploadResult {
  const { adapter } = useAmpContext();
  const [progress, setProgress] = useState(0);
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const upload = useCallback(async (
    file: File, channel: string, opts?: UploadOpts,
  ): Promise<BlobRef> => {
    setUploading(true);
    setProgress(0);
    setError(null);
    try {
      const mergedOpts: UploadOpts = {
        ...opts,
        onProgress: (pct: number) => {
          setProgress(pct);
          opts?.onProgress?.(pct);
        },
      };
      const result = await adapter.upload(file, channel, mergedOpts);
      setProgress(100);
      return result;
    } catch (err) {
      const wrapped = err instanceof Error ? err : new Error(String(err));
      setError(wrapped);
      throw wrapped;
    } finally {
      setUploading(false);
    }
  }, [adapter]);

  return { upload, progress, uploading, error };
}
