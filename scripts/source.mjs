// SPDX-License-Identifier: AGPL-3.0-only
// Export an explicit allowlist without private history, runtime files or caches.
import { readdirSync, lstatSync, readFileSync, writeFileSync, mkdirSync, existsSync } from 'node:fs';
import { resolve, relative } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';

const root = fileURLToPath(new URL('..', import.meta.url));
const dist = resolve(root, 'internal/adminui/dist');
mkdirSync(dist, { recursive: true });
writeFileSync(resolve(dist, 'README.txt'), 'Generated dashboard assets are prepared by make build. Go-only builds serve the preparation page until then.\n');
const roots = ['cmd','internal','migrations','web','deploy','scripts','docs','.github','LICENSES','go.mod','go.sum','LICENSE','NOTICE','THIRD_PARTY_NOTICES.md','README.md','CONTRIBUTING.md','CLA.md','SECURITY.md','versions.yaml','Dockerfile','Makefile','.gitignore','.gitattributes','.dockerignore'];
const excluded = new Set(['node_modules','test-results','playwright-report','.omc','.omo','.codex','.agents','.local']);
const files = [];
function collect(path) {
  if (!existsSync(path)) return;
  const name = relative(root,path).replaceAll('\\','/');
  const info = lstatSync(path);
  if (info.isSymbolicLink()) throw new Error(`Export refuses symlink: ${name}`);
  if (name.startsWith('internal/adminui/dist/') && name !== 'internal/adminui/dist/README.txt') return;
  if (info.isDirectory()) { for (const child of readdirSync(path).sort()) { if(!excluded.has(child) && child!=='.git' && child!=='.env' && !child.startsWith('.env.') && !child.endsWith('.env') && !child.endsWith('.log') && !child.endsWith('.tsbuildinfo')) collect(resolve(path,child)); } }
  else if (info.isFile()) files.push(name);
}
roots.forEach(name=>collect(resolve(root,name))); files.sort();
const hash = createHash('sha256');
for(const name of files) {hash.update(name);hash.update('\0');hash.update(readFileSync(resolve(root,name)));hash.update('\0');}
const revision = `sha256:${hash.digest('hex')}`;
writeFileSync(resolve(root,'.source-revision'),revision+'\n');
const output=process.env.AUTH_SOURCE_ARCHIVE ?? resolve(dist,'source.tar.gz');
execFileSync('tar',['--sort=name','--mtime=2026-09-09T00:00:00Z','--owner=0','--group=0','--numeric-owner','--mode=u+rwX,go+rX,go-w','-czf',output,'--',...files],{cwd:root});
console.log(`Prepared source archive (${files.length} files, ${revision}).`);
