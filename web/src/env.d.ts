/** Build-time settings (see vite.config.ts). */
interface ImportMetaEnv {
  /** The gateway's origin when it is not the page's own; empty: same origin. */
  readonly VITE_API_URL?: string;
  /** "pages" for a static deployment (GitHub Pages). */
  readonly VITE_DEPLOY?: string;
}
