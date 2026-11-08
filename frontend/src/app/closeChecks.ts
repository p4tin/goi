// Checks that run before the window or the project closes. A check returns
// false to cancel (e.g. the user chose not to stop running processes).

export type CloseCheck = () => Promise<boolean>;

const checks: CloseCheck[] = [];

export function addCloseCheck(check: CloseCheck): () => void {
  checks.push(check);
  return () => {
    const i = checks.indexOf(check);
    if (i >= 0) checks.splice(i, 1);
  };
}

/** Runs the checks in order; false as soon as one cancels. */
export async function runCloseChecks(): Promise<boolean> {
  for (const c of [...checks]) {
    try {
      if (!(await c())) return false;
    } catch (err) {
      console.warn('close check', err);
    }
  }
  return true;
}
