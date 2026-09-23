import { createContext, useContext } from "react";
import type { Identity } from "../../shared/api/generated";
export const SessionContext = createContext<Identity | null>(null);
export function useSession() {
  const user = useContext(SessionContext);
  if (!user) throw new Error("Missing session");
  return user;
}
