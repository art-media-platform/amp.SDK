// Marker-filtered AOM staging for pack.sh (one authoritative site).
//
//   node pack-aom-public.mjs <srcDoc> <outDoc>
//
// An operator chapter ships to partners as its PUBLIC subset.  Every
// `## <chapter>.<section>` heading in the source ends in `[PUBLIC]` or
// `[OPERATOR]` (the marker rule is stated in the chapter's own front matter);
// an unmarked section FAILS the pack — never warn-and-skip.  Emitted:
//   - the chapter front matter, its release-marker paragraph replaced by the
//     `(operator)` legend;
//   - every `[PUBLIC]` section, marker stripped;
//   - held-back coordinates rewritten to the greppable `(operator)` token — a
//     `§N.M` naming an OPERATOR section of this chapter, or any coordinate in
//     another operator chapter (§1–§8), with or without an `O{N}` /
//     `O{N}-name.md` prefix.  A coordinate carrying a design-doc prefix
//     (`SD-x.md §8.6`) is left alone: pack-delink.mjs owns those.

import fs from 'node:fs';

const LEGEND = [
  '**Coordinates marked `(operator)`** name procedures held in AMP\'s',
  'Operations Manual and not shipped in this bundle. What follows is the',
  'partner-facing subset of the chapter.',
];

const OPERATOR_CHAPTERS = 8;                         // O1..O8 never ship
const HEADING = /^##\s+(\d+)\.(\d+)\s/;
const MARKED = /^##\s+(\d+)\.(\d+)\s+(.*?)\s*\[(PUBLIC|OPERATOR)\]\s*$/;
const FENCE = /^\s{0,3}(`{3,}|~{3,})/;

const [srcPath, outPath] = process.argv.slice(2);
if (!srcPath || !outPath) {
  console.error('usage: pack-aom-public.mjs <srcDoc> <outDoc>');
  process.exit(2);
}

const src = fs.readFileSync(srcPath, 'utf8');
const lines = src.split('\n');
const errors = [];

// 1. Split the chapter into front matter + marked sections.
const front = [];
const sections = [];                                  // { key, marker, lines }
let current = null;
for (const [idx, line] of lines.entries()) {
  if (HEADING.test(line)) {
    const marked = line.match(MARKED);
    if (!marked) {
      errors.push(
        `${srcPath}:${idx + 1}  section heading carries no ` +
        `[PUBLIC]/[OPERATOR] marker: ${line.trim()}`,
      );
      current = { key: null, marker: 'OPERATOR', lines: [] };
      sections.push(current);
      continue;
    }
    const [, chapter, section, title, marker] = marked;
    current = {
      key: `${chapter}.${section}`,
      marker,
      lines: [`## ${chapter}.${section} ${title}`],
    };
    sections.push(current);
    continue;
  }
  (current ? current.lines : front).push(line);
}

const held = new Set(
  sections.filter((s) => s.marker === 'OPERATOR' && s.key).map((s) => s.key),
);
const shipped = sections.filter((s) => s.marker === 'PUBLIC');
if (shipped.length === 0) {
  errors.push(`${srcPath}  no PUBLIC section to ship`);
}

// 2. Front matter: the marker rule is an authoring rule, not partner text.
const frontOut = [];
for (let idx = 0; idx < front.length; idx++) {
  if (front[idx].startsWith('**Release marker.**')) {
    while (idx < front.length && front[idx].trim() !== '') idx++;
    frontOut.push(...LEGEND, '');
    continue;
  }
  frontOut.push(front[idx]);
}

// 3. Rewrite held-back coordinates outside fenced code.
let rewrites = 0;
function tag(text) {
  return text.replace(
    /([A-Za-z0-9._)\]-]*)(\s*)§(\d+)(?:\.(\d+))?/g,
    (match, prefix, gap, chapter, section) => {
      const docPrefix = /^O\d{1,2}(?:-[a-z0-9-]+\.md)?$/.test(prefix);
      if (!docPrefix && /\.md\)?$/.test(prefix)) return match;  // design-doc coordinate
      const chapterNum = Number(chapter);
      const key = section === undefined ? null : `${chapter}.${section}`;
      const outside = chapterNum <= OPERATOR_CHAPTERS &&
        (docPrefix || key === null || !key.startsWith('4.') || held.has(key));
      if (!outside) return match;
      rewrites++;
      const head = docPrefix ? '' : prefix + gap;
      return `${head}§${chapter}${section === undefined ? '' : `.${section}`}` +
        ' (operator)';
    },
  );
}

function tagBlock(block) {
  let fence = null;
  return block.map((line) => {
    const open = line.match(FENCE);
    if (fence) {
      if (open && open[1][0] === fence[0] && open[1].length >= fence.length) {
        fence = null;
      }
      return line;
    }
    if (open) {
      fence = open[1];
      return line;
    }
    return tag(line);
  });
}

const out = [
  ...tagBlock(frontOut),
  ...shipped.flatMap((section) => tagBlock(section.lines)),
];

if (errors.length > 0) {
  for (const err of errors) console.error(`ERROR: ${err}`);
  process.exit(1);
}

let text = out.join('\n').replace(/\n{3,}$/, '\n');
if (!text.endsWith('\n')) text += '\n';
fs.writeFileSync(outPath, text);
console.log(
  `+++ AOM subset: ${srcPath.split('/').pop()} — ${shipped.length} PUBLIC ` +
  `section(s) shipped, ${held.size} held, ${rewrites} coordinate(s) -> (operator)`,
);
