/* Local math support for the read-only session viewer. MIT, LingNc. */
(function (root, factory) {
  const api = factory();
  if (typeof module === "object" && module.exports) module.exports = api;
  else root.ViewerMath = api;
})(typeof globalThis !== "undefined" ? globalThis : this, function () {
  "use strict";
  const cache = new Map();
  const escape = text => String(text).replace(/[&<>"']/g, c => ({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[c]));
  function escaped(text, position) {
    let slashes = 0;
    while (position > 0 && text[--position] === "\\") slashes++;
    return slashes % 2 === 1;
  }
  // Extract before Markdown sees underscores, stars, pipes or multiline TeX.
  // Fenced/inline code and link destinations remain literal source.
  function extract(text) {
    text = String(text ?? "").replace(/\r\n?/g, "\n");
    let prefix = "\uE000MATH";
    while (text.includes(prefix)) prefix += "X";
    const formulas = [];
    let output = "", index = 0, fence = null;
    while (index < text.length) {
      if (index === 0 || text[index - 1] === "\n") {
        const end = text.indexOf("\n", index);
        const line = text.slice(index, end < 0 ? text.length : end);
        const marker = line.match(/^ {0,3}(`{3,}|~{3,})(.*)$/);
        if (marker && (!fence || (marker[1][0] === fence.char && marker[1].length >= fence.length && !marker[2].trim()))) {
          fence = fence ? null : {char:marker[1][0],length:marker[1].length};
          output += line;
          index += line.length;
          if (text[index] === "\n") { output += "\n"; index++; }
          continue;
        }
        if (fence || /^( {4}|\t)/.test(line)) {
          output += line;
          index += line.length;
          if (text[index] === "\n") { output += "\n"; index++; }
          continue;
        }
      }
      if (text[index] === "`" && !escaped(text, index)) {
        const run = text.slice(index).match(/^`+/)[0];
        let close = text.indexOf(run, index + run.length);
        while (close >= 0 && (text[close - 1] === "`" || text[close + run.length] === "`")) close = text.indexOf(run, close + run.length);
        if (close >= 0) {
          output += text.slice(index, close + run.length);
          index = close + run.length;
          continue;
        }
      }
      if (text.startsWith("](", index)) {
        let end = index + 2, depth = 1;
        for (; end < text.length; end++) {
          if (escaped(text, end)) continue;
          if (text[end] === "(") depth++;
          if (text[end] === ")" && --depth === 0) { end++; break; }
        }
        output += text.slice(index, end);
        index = end;
        continue;
      }
      let open = "", close = "", display = false;
      if (!escaped(text, index)) {
        if (text.startsWith("\\[", index)) { open = "\\["; close = "\\]"; display = true; }
        else if (text.startsWith("\\(", index)) { open = "\\("; close = "\\)"; }
        else if (text.startsWith("$$", index)) { open = close = "$$"; display = true; }
        else if (text[index] === "$" && text[index - 1] !== "$" && !/\s/.test(text[index + 1] || " ")) open = close = "$";
      }
      if (open && formulas.length < 200) {
        const start = index + open.length;
        let end = text.indexOf(close, start);
        while (end >= 0 && (escaped(text, end) || (close === "$" && (/\s/.test(text[end - 1] || " ") || /[\d$]/.test(text[end + 1] || "") || text[end - 1] === "$")))) end = text.indexOf(close, end + close.length);
        if (end >= 0 && end - start <= 20000 && (display || !text.slice(start, end).includes("\n"))) {
          const tex = text.slice(start, end);
          // Do not swallow a closing code fence or another Markdown paragraph.
          if (tex.trim() && !/\n[ \t]*\n|\n {0,3}(?:`{3,}|~{3,})/.test(tex)) {
            const token = prefix + formulas.length + "\uE001";
            formulas.push({token, tex, display, raw:text.slice(index, end + close.length)});
            output += token;
            index = end + close.length;
            continue;
          }
        }
      }
      output += text[index++];
    }
    return {text:output, formulas};
  }
  function renderFormula(formula, katex) {
    if (!katex || typeof katex.renderToString !== "function") return escape(formula.raw);
    const key = (formula.display ? "D" : "I") + formula.tex;
    if (cache.has(key)) {
      const html = cache.get(key);
      cache.delete(key); cache.set(key, html);
      return html;
    }
    let html;
    try {
      html = katex.renderToString(formula.tex, {displayMode:formula.display, throwOnError:true, trust:false, strict:"ignore", maxExpand:1000, maxSize:20, output:"htmlAndMathml", macros:{}});
    } catch (_) {
      return `<span class="math-error" title="公式无法排版，保留原文">${escape(formula.raw)}</span>`;
    }
    cache.set(key, html);
    while (cache.size > 256) cache.delete(cache.keys().next().value);
    return html;
  }
  function renderMarkdown(text, renderer, katex) {
    const parsed = extract(text);
    let html = renderer(parsed.text);
    for (const formula of parsed.formulas) html = html.split(formula.token).join(renderFormula(formula, katex));
    return html;
  }
  return {extract, renderMarkdown, cacheSize:() => cache.size};
});
