import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import vm from 'node:vm';
import { createRequire } from 'node:module';
const require = createRequire(import.meta.url);
const viewerMath = require('./viewer_math.js');
const katex = require('../vendor/katex-0.18.2/katex.min.js');

const html = await readFile(new URL('../viewer.html', import.meta.url), 'utf8');
const script = html.match(/<script>([\s\S]*?)<\/script>/)?.[1];
assert.ok(script, 'inline viewer script exists');
assert.match(html, /<style>[\s\S]*?<\/style>\s*<\/head>/, 'style and document head close before body');
assert.match(html, /<body>[\s\S]*<\/body>\s*<\/html>/, 'viewer body and document close');
new vm.Script(script, { filename: 'viewer.html' });

const elements = new Map();
function element(selector) {
  if (!elements.has(selector)) elements.set(selector, {
    value: '', disabled: false, style: {}, dataset: {}, classList: { add() {}, remove() {}, toggle() {} },
    addEventListener() {}, setAttribute() {}, appendChild() {}, replaceChildren() {},
    querySelectorAll() { return []; },
  });
  return elements.get(selector);
}
let mediaNodes = [];
const observers = [];
class IntersectionObserver {
  constructor(callback) { this.callback = callback; this.targets = new Set(); observers.push(this); }
  observe(node) { this.targets.add(node); }
  unobserve(node) { this.targets.delete(node); }
  disconnect() { this.targets.clear(); }
  intersect() { this.callback([...this.targets].map(target => ({ target, isIntersecting: true }))); }
}
const document = {
  querySelector: element,
  querySelectorAll: selector => selector === '[data-media-ref]' ? mediaNodes : [],
  addEventListener() {},
  createElement: () => element(Symbol()),
};
const window = { ViewerMath: viewerMath, katex };
class ResizeObserver { observe() {} }
const context = { document, window, ResizeObserver, IntersectionObserver, Blob, URL, console, Map, Set, Promise, JSON, String, Number, Math, Date, RegExp };
vm.runInNewContext(script, context, { filename: 'viewer.html' });
const api = window.__viewerTest;
assert.ok(api, 'test hooks initialized');
for (const source of [String.raw`行内 $x_1^2+\alpha$`, String.raw`行内 \(x_1^2+\alpha\)`, '$$\\frac{1}{2}$$', String.raw`\[\int_0^1 x\,dx\]`, '$$\n\\begin{aligned}x&=1\\\\y&=2\\end{aligned}\n$$']) {
  assert.match(api.markdown(source), /class="katex"/, 'all four math delimiters and multiline display math render');
}
const mixed = api.markdown(String.raw`**加粗** 与 $x_i+y_j$，再加 \(a_b\)。`);
assert.match(mixed, /<strong>加粗<\/strong>/);
assert.doesNotMatch(mixed, /<em>i/);
assert.equal((mixed.match(/class="katex"/g) || []).length, 2, 'underscores remain inside math');
for (const literal of ['`$x$`', '```asy\nlabel("$x$",O);\n```', '````text\n```\n$x$\n```\n````', '~~~tex\n\\(x\\)\n~~~', String.raw`只剩闭合符 \) 和 \]`, String.raw`转义 \$x\$`, '价格 $5 与 $10']) {
  assert.doesNotMatch(api.markdown(literal), /class="katex"/, 'source code, escapes, currency and unmatched delimiters stay literal');
}
const outerFence = api.markdown('````text\n```\n$x$\n```\n````\n\n$y$');
assert.equal((outerFence.match(/class="katex"/g) || []).length, 1, 'long outer context fence closes correctly');
assert.match(api.markdown('$\\thiscommanddoesnotexist{x}$'), /math-error/, 'invalid TeX preserves source instead of breaking the message');
assert.doesNotMatch(api.markdown('$\\href{javascript:alert(1)}{click}$'), /href="javascript:/, 'untrusted math cannot create active links');
assert.doesNotMatch(api.markdown('$<script>alert(1)</script>$'), /<script>/, 'TeX input cannot inject raw HTML');
assert.doesNotMatch(viewerMath.renderMarkdown('$x$', source => source, null), /katex/, 'missing renderer retains readable math source');
for (let n=0;n<270;n++) api.markdown(`$x+${n}$`);
assert.ok(viewerMath.cacheSize() <= 256, 'formula cache stays bounded');
api.state.allFiles.length = 0;
api.state.sessionFiles.length = 0;
let transcriptReads = 0;
const directory = { async *entries() {
  yield ['a', { kind: 'directory', async *entries() { yield ['session.jsonl', { kind: 'file', async getFile() { transcriptReads++; return { text: async () => '{"t":"msg"}' }; } }]; } }];
  yield ['b', { kind: 'directory', async *entries() { yield ['session.jsonl', { kind: 'file', async getFile() { transcriptReads++; return { text: async () => '{"t":"msg"}' }; } }]; } }];
} };
await api.walkDirectory(directory);
assert.equal(api.state.sessionFiles.length, 2);
assert.equal(transcriptReads, 0, 'indexing never reads session bodies');
await api.lazyFile(api.state.sessionFiles[0]);
assert.equal(transcriptReads, 1, 'only explicitly selected session body is read');
assert.equal(api.state.sessionFiles[1].file, null, 'non-selected session stays unread');

const rootCompare = api.comparePathsForSession('', [{ path: 'compare/round_0001/comparison.png' }, { path: '预览/submitted.png' }, { path: 'other/compare/comparison.png' }]);
assert.equal(rootCompare.length, 2, 'single-session directory root resolves comparison and final preview');
assert.deepEqual(new Set(Array.from(rootCompare, item => item.path)), new Set([
  'compare/round_0001/comparison.png', '预览/submitted.png',
]), 'root artifacts are found regardless of locale-dependent path order');
const batchCompare = api.comparePathsForSession('figure-6.9', [{ path: 'figure-6.9/compare/round_0001/comparison.png' }, { path: 'figure-6.9/预览/submitted.png' }]);
assert.equal(batchCompare.length, 2);
assert.deepEqual(new Set(Array.from(batchCompare, item => item.path)), new Set([
  'figure-6.9/compare/round_0001/comparison.png', 'figure-6.9/预览/submitted.png',
]), 'batch root resolves artifacts regardless of locale-dependent path order');
const rounds = api.comparePathsForSession('figure-6.9', [
  { path: 'figure-6.9/compare/round_0002/comparison.png' },
  { path: 'figure-6.9/compare/round_0010/comparison.png' },
]);
assert.equal(rounds[0].path, 'figure-6.9/compare/round_0010/comparison.png', 'comparison rounds sort newest-first numerically');

const metadataRows = api.normalizeGoRows([
  { t: 'meta', kind: 'system', tools: [{ function: { name: 'read_image', description: 'Inspect a local image' } }] },
  { t: 'meta', kind: 'runner', text: 'Go runner v3 · go' },
]);
const info = api.renderInfo(metadataRows);
assert.equal(info.title, 'Go runner v3 · go', 'missing model uses runner metadata, not an unknown-model label');
assert.equal(info.subtitle, 'Go runner');
assert.equal(info.tools[0].name, 'read_image', 'Go function.name tool schema is recognized');
assert.equal(info.banner, 'a/session.jsonl');
api.render(metadataRows);
assert.match(elements.get('#latest').innerHTML, /a\/session\.jsonl/);
assert.match(elements.get('#sessionMeta').innerHTML, /Go runner v3/);
assert.doesNotMatch(elements.get('#sessionMeta').innerHTML, /未知模型/);
assert.match(elements.get('#toolList').innerHTML, /read_image/);
assert.equal(api.preferredCompareView([{ name: 'comparison.png' }, { name: 'preview.png' }]), 'preview');
assert.equal(api.preferredCompareView([{ name: 'comparison.png' }]), 'comparison');

const normalized = api.normalizeGoRows([
  { kind: 'system', system: 'private prompt', ts: 't0' },
  { t: 'msg', role: 'assistant', text: 'draw', tool_calls: [{ id: 'call-1', function: { name: 'compile', arguments: '{}' } }] },
  { t: 'msg', role: 'tool', tool_call_id: 'call-1', text: 'compiled' },
  { t: 'checkpoint', ts: 't2', messages: [{ role: 'assistant', content: 'checkpoint text' }] },
  { t: 'delivered', result: { editable_source: 'submitted.lua', best_overlap: 0.92, scale_dev: 0.02 } },
  { t: 'parse_error', __line: 8, error: 'Unexpected token' },
]);
const messages = normalized.filter(x => x.t === 'msg');
assert.equal(messages.length, 4);
assert.equal(messages[0].tool_calls[0].function.name, 'compile');
assert.equal(messages[1].tool_call_id, 'call-1');
assert.match(messages[2].text, /0.92/);
assert.match(messages[2].text, /submitted.lua/);
assert.match(messages[3].text, /第 9 行解析错误/);
const checkpointOnly = api.normalizeGoRows([{ t: 'checkpoint', messages: [{ role: 'assistant', content: 'snapshot only' }] }]);
assert.equal(checkpointOnly.length, 1, 'checkpoint-only import restores its snapshot');
assert.match(checkpointOnly[0].text, /snapshot only/);

assert.equal(api.escapeText('Bearer abc.def api_key=secret123 <script>'), 'Bearer [已隐藏] api_key=[已隐藏] &lt;script&gt;');

const tailRows = api.parseSessionText('{"t":"msg","text":"complete"}\n{"t":"msg"');
assert.equal(tailRows.length, 1, 'incomplete final JSONL line is deferred instead of reported as corruption');
assert.equal(tailRows[0].text, 'complete');
assert.equal(api.parseSessionText('{"t":"msg"}\n{bad}\n').at(-1).t, 'parse_error', 'malformed completed lines remain visible');
let version = 0;
const freshRecord = { file: null, handle: { async getFile() { version++; const snapshot = version; return { text: async () => `snapshot-${snapshot}` }; } } };
assert.equal(await (await api.lazyFile(freshRecord)).text(), 'snapshot-1');
assert.equal(await (await api.lazyFile(freshRecord, true)).text(), 'snapshot-2', 'fresh loads bypass the cached File snapshot');

let reads = 0;
const unused = { path: 'run/media/other.png', handle: { async getFile() { reads++; return { name: 'other.png' }; } } };
const referenced = { path: 'run/media/ref.png', handle: { async getFile() { reads++; return { name: 'ref.png' }; } } };
const meta = { path: 'run/run.json', handle: { async getFile() { reads++; return { name: 'run.json' }; } } };
api.state.allFiles.push(unused, referenced, meta);
for (const rec of [unused, referenced, meta]) api.state.fileByPath.set(rec.path, rec);
await api.warmSelectedFiles('run/', [{ t: 'msg', images: ['file://media/ref.png'] }]);
assert.equal(reads, 1, 'only selected run metadata is read eagerly');
assert.equal(unused.file, undefined, 'unreferenced media stays unread');
assert.equal(referenced.file, undefined, 'referenced media is deferred until its thumbnail enters view');
api.render(Array.from({ length: 4000 }, (_, i) => ({ t: 'msg', role: 'assistant', text: `event ${i}` })));
const timelineHTML = elements.get('#timeline').innerHTML;
assert.ok((timelineHTML.match(/class="entry"/g) || []).length <= 100, 'large transcript initially renders at most 100 timeline events');
assert.match(timelineHTML, /载入更早/, 'older events remain accessible through pagination');
api.render([
  { t: 'msg', role: 'assistant', reasoning_content: '# hidden thought', tool_calls: [{ id: 'direct', function: { name: 'read_image', arguments: '{}' } }] },
  { t: 'msg', role: 'tool', tool_call_id: 'direct', text: 'image result', images: ['file://media/result.png'] },
]);
const lazyDetailsHTML = elements.get('#timeline').innerHTML;
assert.doesNotMatch(lazyDetailsHTML, /<h1>hidden thought<\/h1>/, 'collapsed reasoning is not markdown-rendered');
assert.match(lazyDetailsHTML, /data-media-ref="file:\/\/media\/result\.png"/, 'images directly attached to tool results are shown');
for (const receipt of ['Image returned (call direct).', 'Image returned (tool_call_id=direct).']) {
  api.render([
    { t: 'msg', role: 'assistant', tool_calls: [{ id: 'direct', function: { name: 'read_image', arguments: '{}' } }] },
    { t: 'msg', role: 'tool', tool_call_id: 'direct', text: 'ok' },
    { t: 'msg', role: 'user', text: receipt, images: ['file://media/result.png'] },
  ]);
  assert.equal(api.state.currentEvents.length, 1, 'both image receipt formats attach to their tool without an extra user entry');
  assert.equal(api.state.currentEvents[0].images.length, 1);
}

// These mutable handles model an active run: media appears AFTER the initial walk.
const handleCalls = [], fileReads = [], entryWalks = [];
function dir(name, children = new Map()) {
  return { kind: 'directory', name, children,
    async *entries() { entryWalks.push(name); yield* children; },
    async getDirectoryHandle(child, options) {
      handleCalls.push(['directory', name, child, options.create]);
      const found = children.get(child);
      if (options.create !== false || found?.kind !== 'directory') throw Object.assign(Error(child), { name: 'NotFoundError' });
      return found;
    },
    async getFileHandle(child, options) {
      handleCalls.push(['file', name, child, options.create]);
      const found = children.get(child);
      if (options.create !== false || found?.kind !== 'file') throw Object.assign(Error(child), { name: 'NotFoundError' });
      return found;
    },
  };
}
function file(name, contents = '') {
  return { kind: 'file', async getFile() { fileReads.push(name); return new Blob([contents]); } };
}
const a = dir('a', new Map([['session.jsonl', file('a/session.jsonl')]]));
const b = dir('b', new Map([['session.jsonl', file('b/session.jsonl')]]));
api.directory = dir('root', new Map([['a', a], ['b', b]]));
mediaNodes = [];
await api.loadDirectory();
const scans = entryWalks.length;
const aMedia = dir('a/media', new Map([
  ['new.jfif', file('a/media/new.jfif', 'a')],
  ['unused.png', file('a/media/unused.png', 'unused')],
  ['nested', dir('a/media/nested', new Map([['new.png', file('a/media/nested/new.png', 'nested')]]))],
]));
a.children.set('media', aMedia);
b.children.set('media', dir('b/media', new Map([['new.jfif', file('b/media/new.jfif', 'b')]])));
await api.loadSession(0);
assert.equal(fileReads.filter(x => x.includes('/media/')).length, 0, 'reselecting a session does not discover or read any media eagerly');
assert.equal(entryWalks.length, scans, 'reselecting never rescans the directory');
const [aURL, duplicateURL] = await Promise.all([api.loadMediaRef('file://media/new.jfif'), api.loadMediaRef('file://media/new.jfif')]);
assert.match(aURL, /^blob:/, 'media written after the initial walk is found through session parent handle');
assert.equal(duplicateURL, aURL);
assert.equal(fileReads.filter(x => x === 'a/media/new.jfif').length, 1, 'concurrent thumbnails share a single file read');
assert.equal(api.state.fileByPath.get('a/media/new.jfif').file.type, '', 'empty JFIF MIME is retained without preventing object URL creation');
assert.match(await api.loadMediaRef('file://media/nested/new.png'), /^blob:/, 'nested relative media is discovered by exact path');
assert.equal(fileReads.includes('a/media/unused.png'), false, 'unreferenced media remains unread');
assert.equal(entryWalks.length, scans, 'media discovery never walks directory entries');
const safeCallCount = handleCalls.length;
for (const unsafe of ['file:///tmp/new.jfif', '/media/new.jfif', 'file://media/../new.jfif', 'media/./new.jfif', 'media//new.jfif', 'C:/new.jfif', 'media\\new.jfif', 'https://host/new.jfif', '../new.jfif']) {
  assert.equal(await api.loadMediaRef(unsafe), '', `unsafe reference is rejected: ${unsafe}`);
}
assert.equal(handleCalls.length, safeCallCount, 'unsafe paths perform no filesystem lookup');
assert.equal(await api.loadMediaRef('file://media/no-directory/no.png'), '', 'a genuinely missing directory settles as missing');
assert.equal(await api.loadMediaRef('file://media/no.png'), '', 'a genuinely missing file settles as missing');
assert.ok(handleCalls.every(call => call[3] === false), 'all filesystem lookups explicitly disable creation');
await api.loadSession(1);
const bURL = await api.loadMediaRef('file://media/new.jfif');
assert.notEqual(bURL, aURL, 'same filename in another session has an isolated object URL');
assert.equal(await (await fetch(aURL).catch(() => null)), null, 'switching sessions revokes old object URLs');
assert.equal(await (await fetch(bURL)).text(), 'b', 'same filename resolves within the selected session');

let finishDirectory;
aMedia.getDirectoryHandle = async (name, options) => {
  assert.equal(options.create, false);
  return new Promise(resolve => { finishDirectory = resolve; });
};
await api.loadSession(0);
const staleDirectory = api.loadMediaRef('file://media/slow/new.png');
// Let the first media directory lookup finish, then switch while slow is pending.
await new Promise(resolve => setImmediate(resolve));
await api.loadSession(1);
finishDirectory(dir('slow', new Map([['new.png', file('a/media/slow/new.png')]])));
assert.equal(await staleDirectory, '', 'a session switch cancels pending directory discovery');
assert.equal(api.state.fileByPath.has('a/media/slow/new.png'), false, 'stale discovery cannot add records');
assert.equal(fileReads.includes('a/media/slow/new.png'), false, 'stale discovery does not read a file');

let finishFile;
aMedia.children.set('slow.png', { kind: 'file', getFile() { return new Promise(resolve => { finishFile = resolve; }); } });
await api.loadSession(0);
const oldNode = { dataset: { mediaRef: 'file://media/slow.png' }, isConnected: true };
mediaNodes = [oldNode];
api.render([{ t: 'msg', role: 'user', images: ['file://media/slow.png'] }]);
const oldObserver = observers.at(-1);
oldObserver.intersect();
await new Promise(resolve => setImmediate(resolve));
assert.ok(finishFile, 'viewport intersection starts the referenced file read');
mediaNodes = [];
await api.loadSession(1);
const urlsBefore = api.state.objectURLs.size;
finishFile(new Blob(['old bytes']));
await new Promise(resolve => setImmediate(resolve));
assert.equal(oldNode.outerHTML, undefined, 'stale thumbnail completion cannot update the DOM');
assert.equal(api.state.objectURLs.size, urlsBefore, 'stale file completion cannot create or leak object URLs');
assert.equal(api.state.fileByPath.get('a/media/slow.png').file, null, 'stale file completion cannot write back a cached File');
oldObserver.intersect();
assert.equal(oldObserver.targets.size, 0, 'disconnected old observer remains inert');

const missingNode = { dataset: { mediaRef: 'file://media/absent/no.png' }, isConnected: true };
mediaNodes = [missingNode];
const beforeMissing = handleCalls.length;
api.render([{ t: 'msg', role: 'user', images: ['file://media/absent/no.png'] }]);
const missingObserver = observers.at(-1);
assert.equal(handleCalls.length, beforeMissing, 'render alone performs no media discovery');
missingObserver.intersect();
await new Promise(resolve => setImmediate(resolve));
assert.match(missingNode.outerHTML, /未找到图片/, 'missing media produces a terminal missing placeholder');
const afterMissing = handleCalls.length;
missingObserver.intersect();
await new Promise(resolve => setImmediate(resolve));
assert.equal(handleCalls.length, afterMissing, 'missing media is unobserved and never retried in a loop');

let finishHandle;
const bMedia = b.children.get('media');
bMedia.getFileHandle = async (name, options) => {
  assert.equal(options.create, false);
  return new Promise(resolve => { finishHandle = resolve; });
};
const staleHandle = api.loadMediaRef('file://media/late.png');
await new Promise(resolve => setImmediate(resolve));
assert.ok(finishHandle, 'exact file handle discovery is pending before the directory switch');
mediaNodes = [];
api.directory = dir('replacement');
await api.loadDirectory();
finishHandle(file('b/media/late.png', 'stale'));
assert.equal(await staleHandle, '', 'switching the authorized root cancels pending file handle discovery');
assert.equal(api.state.fileByPath.size, 0, 'a stale handle cannot enter the replacement directory index');
assert.equal(fileReads.includes('b/media/late.png'), false, 'stale file handles never read file contents');
console.log(`viewer parser, fresh-file, late-media, race, isolation, lazy-read, and bounded-timeline checks passed (${Intl.DateTimeFormat().resolvedOptions().locale})`);
