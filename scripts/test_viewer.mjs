import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import vm from 'node:vm';

const html = await readFile(new URL('../viewer.html', import.meta.url), 'utf8');
const script = html.match(/<script>([\s\S]*?)<\/script>/)?.[1];
assert.ok(script, 'inline viewer script exists');
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
const window = {};
class ResizeObserver { observe() {} }
const context = { document, window, ResizeObserver, Blob, URL, console, Map, Set, Promise, JSON, String, Number, Math, Date, RegExp };
vm.runInNewContext(script, context, { filename: 'viewer.html' });
const api = window.__viewerTest;
assert.ok(api, 'test hooks initialized');
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
assert.equal(rootCompare[0].path, 'compare/round_0001/comparison.png');
assert.equal(rootCompare[1].path, '预览/submitted.png');
const batchCompare = api.comparePathsForSession('figure-6.9', [{ path: 'figure-6.9/compare/round_0001/comparison.png' }, { path: 'figure-6.9/预览/submitted.png' }]);
assert.equal(batchCompare.length, 2);
assert.equal(batchCompare[0].path, 'figure-6.9/compare/round_0001/comparison.png', 'batch root resolves nested session artifacts');
assert.equal(batchCompare[1].path, 'figure-6.9/预览/submitted.png');

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

let reads = 0;
const unused = { path: 'run/media/other.png', handle: { async getFile() { reads++; return { name: 'other.png' }; } } };
const referenced = { path: 'run/media/ref.png', handle: { async getFile() { reads++; return { name: 'ref.png' }; } } };
const meta = { path: 'run/run.json', handle: { async getFile() { reads++; return { name: 'run.json' }; } } };
api.state.allFiles.push(unused, referenced, meta);
await api.warmSelectedFiles('run/', [{ t: 'msg', images: ['file://media/ref.png'] }]);
assert.equal(reads, 2, 'only metadata and referenced media are read');
assert.equal(unused.file, undefined, 'unreferenced media stays unread');
assert.ok(referenced.file);
console.log('viewer parser and lazy-file checks passed');
