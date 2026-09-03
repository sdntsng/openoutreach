export const DEFAULT_SEQUENCE = `name: outreach
defaults:
  from_name: "You"
steps:
  - step: 1
    delay: 0
    subject: "Quick question for {{company}}"
    body: |
      Hi {{first_name}},

      Wanted to reach out about {{company}}.
  - step: 2
    delay: 3
    body: |
      Hi {{first_name}},

      Following up once in case this got buried.
`;

export function replyRateLabel(n?: number): string {
  if (n == null || Number.isNaN(n)) return "—";
  const v = n <= 1 && n > 0 && n < 1 ? n * 100 : n;
  return `${v.toFixed(1)}%`;
}

export function isHot(classification?: string): boolean {
  const c = (classification || "").toLowerCase();
  return c === "positive" || c === "interested" || c === "hot";
}
