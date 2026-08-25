import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// The built assets are embedded into the sbnn binary (see web.go), so the
// output has to stay in dist/. During development everything under /_ is
// proxied to a running sbnn server (sbnn --foreground).
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    // Pages written by `sbnn export` are opened as file:// documents or
    // embedded in a host page, where an emitted asset URL like
    // /assets/foo.woff2 resolves against the wrong root and 404s. The icon
    // font is the only asset the stylesheet still reaches for, so it is
    // inlined as a data URL whatever its size and the built CSS carries it.
    // Everything else keeps Vite's default threshold: images in particular
    // must stay separate files, since the payload already inlines the ones
    // it needs and a bundle-wide inline would bloat the page.
    assetsInlineLimit: (filePath: string) =>
      filePath.endsWith('.woff2') ? true : undefined,
  },
  server: {
    fs: {
      strict: false,
    },
    proxy: {
      '/_': {
        target: 'http://localhost:6280',
        changeOrigin: false,
      },
    },
  },
})
