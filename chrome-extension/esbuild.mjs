import * as esbuild from 'esbuild';
import { cpSync, mkdirSync, rmSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = dirname(fileURLToPath(import.meta.url));
const dist = join(root, 'dist');
const watch = process.argv.includes('--watch');
const dev = process.argv.includes('--dev');

function copyStatic() {
  cpSync(join(root, 'manifest.json'), join(dist, 'manifest.json'));
  cpSync(join(root, 'popup/popup.html'), join(dist, 'popup.html'));
  cpSync(join(root, 'popup/popup.css'), join(dist, 'popup.css'));
  cpSync(join(root, 'assets'), join(dist, 'assets'), { recursive: true });
}

const buildOptions = {
  entryPoints: {
    background: join(root, 'background.ts'),
    popup: join(root, 'popup/popup.ts'),
  },
  bundle: true,
  outdir: dist,
  format: 'iife',
  platform: 'browser',
  target: ['es2020'],
  sourcemap: dev,
  minify: !dev,
  logLevel: 'info',
};

async function main() {
  rmSync(dist, { recursive: true, force: true });
  mkdirSync(dist, { recursive: true });

  if (watch) {
    const ctx = await esbuild.context(buildOptions);
    await ctx.watch();
    copyStatic();
    console.log('[esbuild] watching…');
    return;
  }

  await esbuild.build(buildOptions);
  copyStatic();
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
