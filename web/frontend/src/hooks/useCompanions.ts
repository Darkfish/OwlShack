import { useEffect, useMemo, useState } from "react";

import { companionIdFromRef } from "@/lib/companionRef";

// A companion reference as exposed by GET /api/companions.
export interface CompanionRef {
  id: number;
  name: string;
  pubkey?: string;
}

// Module-level stale-while-revalidate cache: many pages fetch /api/companions independently.
let cache: CompanionRef[] | null = null;
let inflight: Promise<CompanionRef[]> | null = null;
const subscribers = new Set<(c: CompanionRef[]) => void>();

function fetchCompanions(): Promise<CompanionRef[]> {
  if (inflight) return inflight;
  inflight = fetch("/api/companions")
    .then((r) => (r.ok ? r.json() : []))
    .then((cs: CompanionRef[]) => {
      cache = cs || [];
      subscribers.forEach((fn) => fn(cache!));
      return cache;
    })
    .catch(() => cache ?? [])
    .finally(() => {
      inflight = null;
    });
  return inflight;
}

export function useCompanions(): CompanionRef[] {
  const [companions, setCompanions] = useState<CompanionRef[]>(cache ?? []);

  useEffect(() => {
    let active = true;
    const update = (c: CompanionRef[]) => {
      if (active) setCompanions(c);
    };
    subscribers.add(update);
    // Revalidate on every mount, so a mutation elsewhere self-heals.
    fetchCompanions().then(update);
    return () => {
      subscribers.delete(update);
      active = false;
    };
  }, []);

  return companions;
}

// useCompanionRef reads the :ref route segment. Use `ref` for links and API paths, where it must
// survive a rename, and `name` wherever a person reads it or it is compared against a sender - the
// two stop being interchangeable the moment a companion is renamed.
//
// `name` is empty until the companion list resolves, and stays the segment itself for a plain-name
// link made before refs existed.
export function useCompanionRef(ref: string | undefined): {
  ref: string;
  id: number | null;
  name: string;
} {
  const companions = useCompanions();
  return useMemo(() => {
    const seg = ref ?? "";
    const id = companionIdFromRef(seg);
    if (id == null) return { ref: seg, id: null, name: seg };
    return {
      ref: seg,
      id,
      name: companions.find((c) => c.id === id)?.name ?? "",
    };
  }, [ref, companions]);
}
