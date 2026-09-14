// A companion's URL token: its id, plus a slug of its name so the link stays readable. The id is
// the authority and the slug is decoration, so a rename never breaks a link the user already has.
// The backend accepts this, a bare id, or a plain name, the last for links made before this existed.

export function companionRef(c: { id: number; name: string }): string {
  const slug = c.name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
  return slug ? `${c.id}-${slug}` : String(c.id);
}

export function companionIdFromRef(ref: string): number | null {
  const m = /^(\d+)(?:-|$)/.exec(ref);
  return m ? Number(m[1]) : null;
}

export function companionPath(c: { id: number; name: string }, rest = ""): string {
  return `/companions/${companionRef(c)}${rest}`;
}

// Whether a :ref path segment addresses this companion, by id for a ref and by name for a link
// made before refs existed.
export function refMatches(
  seg: string,
  c: { id: number; name: string },
): boolean {
  const id = companionIdFromRef(seg);
  return id != null ? id === c.id : seg === c.name;
}
