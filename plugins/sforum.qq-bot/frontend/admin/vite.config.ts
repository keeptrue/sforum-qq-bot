import { defineConfig } from 'vite'

// The Host never builds plugin sources: it serves the committed bundle from
// frontend/admin/dist as one ES module plus one stylesheet. Keep both names
// stable and unhashed so the manifest paths never move between builds.
export default defineConfig({
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    sourcemap: false,
    minify: 'oxc',
    cssCodeSplit: false,
    lib: {
      entry: 'src/settings.ts',
      formats: ['es'],
      fileName: () => 'settings.mjs',
      cssFileName: 'settings'
    }
  }
})
