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
const document = {
  querySelector: element,
  querySelectorAll: () => [],
  addEventListener() {},
  createElement: () => element(Symbol()),
};
const window = { ViewerMath: viewerMath, katex };
class ResizeObserver { observe() {} }
const context = { document, window, ResizeObserver, Blob, URL, console, Map, Set, Promise, JSON, String, Number, Math, Date, RegExp };
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
console.log(`viewer parser, fresh-file, partial-write, and bounded-timeline checks passed (${Intl.DateTimeFormat().resolvedOptions().locale})`);
