import { useEffect, useState } from "react";
import type {
  Document,
  DocumentContent,
  DocumentRevision,
  DocumentSaved,
} from "../../shared/api/generated";
import { post, request } from "../../shared/api/client";
export function useDocumentDraft(document: Document) {
  const [base, setBase] = useState<{
    document: Document;
    revision: DocumentRevision;
  }>();
  const [draft, setDraft] = useState<DocumentContent>();
  const [error, setError] = useState<unknown>();
  const [remote, setRemote] = useState<DocumentRevision>();
  const [loading, setLoading] = useState(true);
  useEffect(() => {
    if (base || !document.current_revision_id) {
      setLoading(false);
      return;
    }
    let live = true;
    setLoading(true);
    request<DocumentRevision>(
      `/documents/${document.id}/revisions/${document.current_revision_id}`,
    )
      .then((revision) => {
        if (live) {
          setBase({ document, revision });
          setDraft(revision.content);
          setLoading(false);
          setError(undefined);
        }
      })
      .catch((e) => {
        if (live) {
          setError(e);
          setLoading(false);
        }
      });
    return () => {
      live = false;
    };
  }, [document.id, document.current_revision_id, base]);
  const dirty =
    !!base && JSON.stringify(draft) !== JSON.stringify(base.revision.content);
  async function save() {
    if (!base || !draft) throw new Error("No document");
    if (!dirty) return base.revision;
    try {
      const saved = await post<DocumentSaved>(
        "/documents/" + document.id + "/revisions",
        {
          expected_version: base.document.version,
          base_revision_id: base.revision.id,
          content: draft,
        },
      );
      setBase(saved);
      setDraft(saved.revision.content);
      setRemote(undefined);
      setError(undefined);
      return saved.revision;
    } catch (e) {
      setError(e);
      throw e;
    }
  }
  async function compare() {
    const latest = await request<Document>("/documents/" + document.id);
    if (latest.current_revision_id) {
      const revision = await request<DocumentRevision>(
        `/documents/${document.id}/revisions/${latest.current_revision_id}`,
      );
      setRemote(revision);
    }
  }
  async function acceptRemote() {
    const latest = await request<Document>("/documents/" + document.id);
    if (!latest.current_revision_id) return;
    const revision = await request<DocumentRevision>(
      `/documents/${document.id}/revisions/${latest.current_revision_id}`,
    );
    setBase({ document: latest, revision });
    setDraft(revision.content);
    setRemote(undefined);
    setError(undefined);
  }
  return {
    draft,
    setDraft,
    base,
    dirty,
    error,
    loading,
    remote,
    save,
    compare,
    acceptRemote,
  };
}
