/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** Prefix API; default `/api/v1` (satu origin lewat proxy/rewrite). */
  readonly VITE_API_BASE_URL?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
