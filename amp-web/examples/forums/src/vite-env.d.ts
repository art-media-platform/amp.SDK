/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_AMP_VAULT_URL?: string;
  readonly VITE_AMP_PLANET_TAG?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
