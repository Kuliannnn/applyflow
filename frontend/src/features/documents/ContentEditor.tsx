import { AutoTextarea } from "../../shared/ui/AutoTextarea";
import type { DocumentContent } from "../../shared/api/generated";
export function ContentEditor({
  value,
  onChange,
  disabled = false,
}: {
  value: DocumentContent;
  onChange: (v: DocumentContent) => void;
  disabled?: boolean;
}) {
  if (value.kind === "cover_letter")
    return (
      <div className="document-body">
        <label>
          Salutation
          <input
            disabled={disabled}
            value={value.salutation}
            onChange={(e) => onChange({ ...value, salutation: e.target.value })}
          />
        </label>
        {value.paragraphs.map((p, i) => (
          <label key={i}>
            Paragraph {i + 1}
            <AutoTextarea
              disabled={disabled}
              rows={Math.max(3, Math.ceil(p.text.length / 75))}
              value={p.text}
              onChange={(e) =>
                onChange({
                  ...value,
                  paragraphs: value.paragraphs.map((p, n) =>
                    n === i ? { ...p, text: e.target.value } : p,
                  ),
                })
              }
            />
          </label>
        ))}
        <label>
          Closing
          <AutoTextarea
            disabled={disabled}
            rows={2}
            value={value.closing}
            onChange={(e) => onChange({ ...value, closing: e.target.value })}
          />
        </label>
      </div>
    );
  return (
    <div className="document-body">
      <label>
        Document title
        <input
          className="document-title"
          disabled={disabled}
          value={value.title}
          onChange={(e) => onChange({ ...value, title: e.target.value })}
        />
      </label>
      {value.sections.map((s, i) => (
        <section className="document-section" key={i}>
          <label>
            Section title
            <input
              className="section-title"
              disabled={disabled}
              value={s.heading}
              onChange={(e) =>
                onChange({
                  ...value,
                  sections: value.sections.map((s, n) =>
                    n === i ? { ...s, heading: e.target.value } : s,
                  ),
                })
              }
            />
          </label>
          {s.items.map((item, j) => (
            <label key={j}>
              Experience / fact {j + 1}
              <AutoTextarea
                disabled={disabled}
                rows={Math.max(2, Math.ceil(item.text.length / 75))}
                value={item.text}
                onChange={(e) =>
                  onChange({
                    ...value,
                    sections: value.sections.map((s, n) =>
                      n === i
                        ? {
                            ...s,
                            items: s.items.map((v, k) =>
                              k === j ? { ...v, text: e.target.value } : v,
                            ),
                          }
                        : s,
                    ),
                  })
                }
              />
            </label>
          ))}
        </section>
      ))}
    </div>
  );
}
export function ContentPreview({ value }: { value: DocumentContent }) {
  return value.kind === "resume" ? (
    <div className="revision-preview">
      <h3>{value.title}</h3>
      {value.sections.map((s, i) => (
        <section key={i}>
          <h4>{s.heading}</h4>
          {s.items.map((p, j) => (
            <p key={j}>{p.text}</p>
          ))}
        </section>
      ))}
    </div>
  ) : (
    <div className="revision-preview">
      <p>{value.salutation}</p>
      {value.paragraphs.map((p, i) => (
        <p key={i}>{p.text}</p>
      ))}
      <p>{value.closing}</p>
    </div>
  );
}
