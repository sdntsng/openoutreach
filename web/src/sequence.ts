export type SequenceStep = {
  step: number;
  delay: number;
  subject: string;
  body: string;
};

export type SequenceDoc = {
  name: string;
  from_name: string;
  steps: SequenceStep[];
};

export type RenderedEmail = {
  step: number;
  delay: number;
  subject: string;
  body: string;
  send_at?: string;
  send_note?: string;
};

function unindentBlock(s: string): string {
  const lines = s.replace(/\r\n/g, "\n").replace(/\n$/, "").split("\n");
  let min = 99;
  for (const ln of lines) {
    if (!ln.trim()) continue;
    const m = ln.match(/^(\s*)/);
    if (m && m[1].length < min) min = m[1].length;
  }
  if (min === 99) return s.trimEnd();
  return lines.map((ln) => (ln.length >= min ? ln.slice(min) : ln)).join("\n");
}

function yamlScalar(s: string): string {
  const t = s.trim();
  if (!t) return `""`;
  const need =
    /[:#{}[\]&*!|>'"%@`,]/.test(t) || t.includes("\n") || t.startsWith(" ") || t === "true" || t === "false" || t === "null";
  if (!need) return t;
  return JSON.stringify(t);
}

/** Same format as hosted SequenceToYAML / ParseSequenceFromBytes. */
export function sequenceToYAML(doc: SequenceDoc): string {
  const name = doc.name || "outreach";
  const from = doc.from_name || "You";
  let y = `name: ${yamlScalar(name)}\ndefaults:\n  from_name: ${yamlScalar(from)}\nsteps:\n`;
  doc.steps.forEach((st, i) => {
    const n = st.step || i + 1;
    y += `  - step: ${n}\n    delay: ${st.delay || 0}\n`;
    if ((st.subject || "").trim()) y += `    subject: ${yamlScalar(st.subject)}\n`;
    y += `    body: |\n`;
    const body = (st.body || "").replace(/\n$/, "");
    if (!body) {
      y += `      \n`;
      return;
    }
    for (const line of body.split("\n")) y += `      ${line}\n`;
  });
  return y;
}

export function parseSequenceYAML(raw: string): SequenceDoc | null {
  const yaml = (raw || "").replace(/\r\n/g, "\n").trim();
  if (!yaml) return { name: "outreach", from_name: "You", steps: [] };
  if (/\n\s+variants:\s*\n/.test("\n" + yaml)) return null;
  const nameM = yaml.match(/(?:^|\n)name:\s*(?:"([^"]*)"|'([^']*)'|([^\n]+))/);
  const fromM = yaml.match(/(?:^|\n)defaults:\s*\n(?:[ \t]+from_name:\s*(?:"([^"]*)"|'([^']*)'|([^\n]+)))/);
  const name = (nameM?.[1] || nameM?.[2] || nameM?.[3] || "outreach").trim();
  const fromName = (fromM?.[1] || fromM?.[2] || fromM?.[3] || "You").trim();
  const seqIdx = yaml.search(/(?:^|\n)steps:\s*\n/);
  if (seqIdx < 0) return { name, from_name: fromName, steps: [] };
  const seqBlock = yaml.slice(seqIdx);
  const itemRe = /\n[ \t]+-[ \t]+step:\s*\d+/g;
  const indexes: number[] = [];
  let m: RegExpExecArray | null;
  while ((m = itemRe.exec(seqBlock))) indexes.push(m.index);
  if (!indexes.length) return { name, from_name: fromName, steps: [] };
  const steps: SequenceStep[] = [];
  for (let i = 0; i < indexes.length; i++) {
    const chunk = seqBlock.slice(indexes[i], i + 1 < indexes.length ? indexes[i + 1] : undefined);
    const stepM = chunk.match(/step:\s*(-?\d+)/);
    const delayM = chunk.match(/delay:\s*(-?\d+)/);
    const subM = chunk.match(/subject:\s*(?:"([^"]*)"|'([^']*)'|([^\n]+))/);
    const bodyM = chunk.match(/body:\s*\|\s*\n([\s\S]*)/);
    steps.push({
      step: stepM ? parseInt(stepM[1], 10) : i + 1,
      delay: delayM ? parseInt(delayM[1], 10) : 0,
      subject: (subM?.[1] || subM?.[2] || subM?.[3] || "").trim(),
      body: bodyM ? unindentBlock(bodyM[1]) : "",
    });
  }
  return { name, from_name: fromName, steps };
}

export function defaultSequence(): SequenceDoc {
  return {
    name: "outreach",
    from_name: "You",
    steps: [
      {
        step: 1,
        delay: 0,
        subject: "Quick question, {{first_name}}",
        body: "Hi {{first_name}},\n\nI noticed {{company}} and thought it was worth a short note.\n\nWould you be open to a brief conversation?\n\nBest",
      },
    ],
  };
}

export function renderFields(text: string, fields: Record<string, string>): string {
  return (text || "").replace(/\{\{\s*(\w+)\s*\}\}/g, (_, k: string) => fields[k] ?? "");
}

export const SAMPLE_FIELDS: Record<string, string> = {
  first_name: "Ada",
  last_name: "Lovelace",
  company: "Acme",
  email: "ada@acme.com",
  domain: "acme.com",
};
