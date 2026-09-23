/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_PAYMENTS_API_URL?: string
  readonly VITE_ANALYTICS_API_URL?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
