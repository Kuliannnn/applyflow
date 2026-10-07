import { prepareResumeSource } from "../resumes/prepare-source";
import { useEffect, useState } from "react";
import { flushSync } from "react-dom";
import { Commands } from "../../shared/api/client";
import type { Workspace, WorkspaceList } from "../../shared/api/generated";
import { useResource } from "../../shared/api/use-resource";
import { useAction, useUnsaved } from "../../shared/ui/common";
import { generate, loadInput, saveInput } from "./api";
import type { GenerationChoice } from "./GenerationMode";
import { useSession } from "../auth/Session";
export function useCreateWorkflow(id?: string) {
  const user = useSession();
  const [choice, setChoice] = useState<GenerationChoice>({ mode: "mock" });
  const [commands] = useState(() => new Commands());
  const [w, setW] = useState<Workspace>();
  const [text, setText] = useState(""),
    [selected, setSelected] = useState("");
  const [company, setCompany] = useState(""),
    [role, setRole] = useState("");
  const [unsupported, setUnsupported] = useState(false),
    [ready, setReady] = useState(!id),
    [dirty, setDirty] = useState(false);
  const action = useAction();
  const recent = useResource<WorkspaceList>(!id ? "/workspaces?limit=5" : null);
  useUnsaved(dirty);
  useEffect(() => {
    if (!id) return;
    let live = true;
    void loadInput(id)
      .then((r) => {
        if (!live) return;
        setW(r.workspace);
        setSelected(r.workspace.resume_revision_id ?? "");
        setText(
          r.job?.job_description ??
            (r.source?.source.kind === "text" ? r.source.source.text : ""),
        );
        setCompany(r.job?.company ?? "");
        setRole(r.job?.role_title ?? "");
        setReady(true);
      })
      .catch((e) => {
        if (live)
          void action.run(async () => {
            throw e;
          });
      });

    return () => {
      live = false;
    };
  }, [id]);
  const url = /^https?:\/\/\S+$/.test(text.trim());
  function submit() {
    void action.run(async () => {
      const prepared = await prepareResumeSource(selected);
      setSelected(prepared);
      const saved = await saveInput(commands, w, text, prepared, setW);
      setW(saved);
      history.replaceState(null, "", "/create/" + saved.id);
      await generate(
        commands,
        saved,
        company,
        role,
        text,
        user.profile_version,
        choice,
      );
      flushSync(() => setDirty(false));
      location.assign("/studio/" + saved.id);
    });
  }
  return {
    choice,
    setChoice,
    text,
    setText,
    selected,
    setSelected,
    company,
    setCompany,
    role,
    setRole,
    unsupported,
    setUnsupported,
    ready,
    setDirty,
    action,
    recent,
    url,
    submit,
  };
}
