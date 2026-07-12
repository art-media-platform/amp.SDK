// Post-staging de-link pass for pack.sh (one authoritative site).
//
//   node pack-delink.mjs <stageDir> <aomSrcDir>
//
// For every .md staged into the bundle, resolve each relative markdown
// link against the stage tree (anchors stripped; absolute URLs and code
// spans skipped):
//   - target ships in the bundle       -> keep as-is
//   - target is an unshipped AOM doc   -> rewrite `[Text](path)` to the
//     canonical greppable token `Text (internal)`
//   - any other missing target         -> ERROR, fail the pack (file:line)
// A de-link candidate on an instruction-class line (preread / must read /
// required reading / read ... first) also FAILS the pack: a token cannot
// rescue a sentence instructing the reader to read an absent doc — reword
// at source (README "Authoring Notes").
//
// Also verifies the (internal) legend line is present in the staged
// top-level README when AOM docs ship.

import fs from 'node:fs';
import path from 'node:path';

const LEGEND =
  'References marked (internal) name AMP-internal design docs not ' +
  'shipped in this bundle — background provenance, not required reading.';

const INSTRUCTION_RE = /preread|must[- ]read|required reading|read .*first/i;
const LINK_RE = /\[([^\]]*)\]\(([^()]*)\)/g;

const [stageDir, aomSrcDir] = process.argv.slice(2);
if (!stageDir) {
  console.error('usage: pack-delink.mjs <stageDir> <aomSrcDir>');
  process.exit(2);
}
const STAGE = path.resolve(stageDir);
const AOM_SRC = aomSrcDir && fs.existsSync(aomSrcDir)
  ? path.resolve(aomSrcDir)
  : null;

function mdFiles(dir) {
  const out = [];
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (entry.name === 'node_modules') continue;
      out.push(...mdFiles(full));
    } else if (entry.name.endsWith('.md')) {
      out.push(full);
    }
  }
  return out;
}

// Char ranges covered by fenced code blocks or inline code spans; links
// inside them are not candidates.
function maskedRanges(text) {
  const ranges = [];
  const lines = text.split('\n');
  let pos = 0;
  let fence = null;                       // open fence marker, or null
  let fenceStart = 0;
  for (const line of lines) {
    const open = line.match(/^\s{0,3}(`{3,}|~{3,})/);
    if (fence) {
      if (open && open[1][0] === fence[0] && open[1].length >= fence.length) {
        ranges.push([fenceStart, pos + line.length]);
        fence = null;
      }
    } else if (open) {
      fence = open[1];
      fenceStart = pos;
    }
    pos += line.length + 1;
  }
  if (fence) ranges.push([fenceStart, text.length]);   // unterminated fence

  const inMask = (idx) => ranges.some(([a, b]) => idx >= a && idx < b);
  for (const span of text.matchAll(/(`{1,2})[^`]+\1/g)) {
    if (!inMask(span.index)) {
      ranges.push([span.index, span.index + span[0].length]);
    }
  }
  return ranges;
}

const rewrites = [];        // { file, line, text, target }
const errors = [];

function delinkFile(file) {
  const rel = path.relative(STAGE, file);
  const src = fs.readFileSync(file, 'utf8');
  const masks = maskedRanges(src);
  const masked = (idx) => masks.some(([a, b]) => idx >= a && idx < b);
  const lineOf = (idx) => src.slice(0, idx).split('\n').length;
  const lineText = (idx) => {
    const start = src.lastIndexOf('\n', idx - 1) + 1;
    const end = src.indexOf('\n', idx);
    return src.slice(start, end === -1 ? src.length : end);
  };

  const out = src.replace(LINK_RE, (match, text, rawTarget, idx) => {
    if (masked(idx)) return match;
    let target = rawTarget.trim().split(/\s+/)[0];      // drop "title"
    if (target.startsWith('<') && target.endsWith('>')) {
      target = target.slice(1, -1);
    }
    if (
      target === '' ||
      target.startsWith('#') ||                          // same-doc anchor
      target.startsWith('//') ||
      /^[a-z][a-z0-9+.-]*:/i.test(target)                // absolute URL
    ) {
      return match;
    }
    target = target.split('#')[0];                       // strip fragment
    try { target = decodeURIComponent(target); } catch { /* keep raw */ }

    const resolved = path.resolve(path.dirname(file), target);
    if (fs.existsSync(resolved)) return match;           // ships in bundle

    const stageRel = path.relative(STAGE, resolved);
    const aomRel = stageRel.startsWith('AOM' + path.sep)
      ? stageRel.slice(4)
      : null;
    const isAOM = aomRel !== null &&
      (AOM_SRC === null || fs.existsSync(path.join(AOM_SRC, aomRel)));
    const at = `${rel}:${lineOf(idx)}`;
    if (!isAOM) {
      errors.push(`dangling link: ${at}  [${text}](${rawTarget})`);
      return match;
    }
    if (INSTRUCTION_RE.test(lineText(idx))) {
      errors.push(
        `instruction-class line cites unshipped doc: ${at}  ` +
        `[${text}](${rawTarget}) — reword at source ` +
        `(README "Authoring Notes")`,
      );
      return match;
    }
    rewrites.push({ file: rel, line: lineOf(idx), text, target });
    return `${text} (internal)`;
  });

  if (out !== src) fs.writeFileSync(file, out);
}

for (const file of mdFiles(STAGE)) delinkFile(file);

// Legend: the bundle README carries the (internal) legend line whenever
// AOM docs ship — authored in the source README, verified here.
if (fs.existsSync(path.join(STAGE, 'AOM'))) {
  const readme = path.join(STAGE, 'README.md');
  if (!fs.readFileSync(readme, 'utf8').includes(LEGEND)) {
    errors.push(
      `legend line missing from README.md — restore under ` +
      `"Design References (AOM)": "${LEGEND}"`,
    );
  }
}

if (errors.length > 0) {
  for (const err of errors) console.error(`ERROR: ${err}`);
  process.exit(1);
}

const files = new Set(rewrites.map((entry) => entry.file));
console.log(
  `+++ de-link: ${rewrites.length} AOM ref(s) -> (internal) ` +
  `across ${files.size} file(s)`,
);
for (const { file, line, text, target } of rewrites) {
  console.log(`    ${file}:${line}  [${text}] -> ${target} (internal)`);
}
