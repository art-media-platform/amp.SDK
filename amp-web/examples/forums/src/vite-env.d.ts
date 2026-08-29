/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_AMP_VAULT_URL?: string;
  readonly VITE_AMP_PLANET_TAG?: string;
  /** REQUIRED: the public share planet anonymous share reads ride (SKILL §10 item 3). */
  readonly VITE_AMP_PUBLIC_SHARE_PLANET_TAG: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
