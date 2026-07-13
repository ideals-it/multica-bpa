import { ipcMain, shell } from "electron";
import { realpath } from "fs/promises";
import { isAbsolute, relative, resolve, sep } from "path";
import { getDesktopDaemonApiCredentials } from "./daemon-manager";

type LocalArtifactAction = "open" | "reveal";

interface LocalArtifactResponse {
  work_dir: string;
  relative_path: string;
}

interface OpenLocalArtifactResult {
  ok: boolean;
  error?: string;
}

function isRelativeArtifactPath(path: string): boolean {
  return Boolean(path) && !isAbsolute(path) && path !== "." && path !== ".." && !path.startsWith(`..${sep}`);
}

// resolveLocalArtifactPath is deliberately filesystem-only so the containment
// boundary is independently testable. The server separately proves that the
// work directory belongs to the requested task and workspace.
export async function resolveLocalArtifactPath(workDir: string, relativePath: string): Promise<string> {
  if (!isAbsolute(workDir) || !isRelativeArtifactPath(relativePath)) {
    throw new Error("artifact path must be relative to its task work directory");
  }
  const root = await realpath(workDir);
  const target = await realpath(resolve(root, relativePath));
  const fromRoot = relative(root, target);
  if (fromRoot === "" || fromRoot === ".." || fromRoot.startsWith(`..${sep}`) || isAbsolute(fromRoot)) {
    throw new Error("artifact path resolves outside its task work directory");
  }
  return target;
}

async function fetchTaskArtifact(taskId: string, artifactPath: string): Promise<LocalArtifactResponse> {
  const credentials = await getDesktopDaemonApiCredentials();
  if (!credentials) throw new Error("Desktop is not connected to a local Multica runtime");
  const url = new URL(`/api/daemon/tasks/${encodeURIComponent(taskId)}/local-artifact`, credentials.serverUrl);
  url.searchParams.set("artifact_path", artifactPath);
  const response = await fetch(url, {
    headers: { Authorization: `Bearer ${credentials.token}` },
  });
  if (!response.ok) throw new Error("Local artifact is unavailable for this task");
  const body = (await response.json()) as Partial<LocalArtifactResponse>;
  if (typeof body.work_dir !== "string" || typeof body.relative_path !== "string") {
    throw new Error("Local artifact response is invalid");
  }
  return { work_dir: body.work_dir, relative_path: body.relative_path };
}

async function openLocalArtifact(
  taskId: string,
  action: LocalArtifactAction,
  artifactPath: string,
): Promise<OpenLocalArtifactResult> {
  if (!/^[0-9a-f]{8}-[0-9a-f-]{28}$/i.test(taskId) || (action !== "open" && action !== "reveal") || !isRelativeArtifactPath(artifactPath)) {
    return { ok: false, error: "Invalid local artifact link" };
  }
  try {
    const artifact = await fetchTaskArtifact(taskId, artifactPath);
    const target = await resolveLocalArtifactPath(artifact.work_dir, artifact.relative_path);
    if (action === "reveal") {
      shell.showItemInFolder(target);
      return { ok: true };
    }
    const error = await shell.openPath(target);
    return error ? { ok: false, error } : { ok: true };
  } catch (error) {
    return { ok: false, error: error instanceof Error ? error.message : "Unable to open local artifact" };
  }
}

export function setupLocalArtifact(): void {
  ipcMain.handle(
    "local-artifact:open",
    (_event, taskId: string, action: LocalArtifactAction, artifactPath: string): Promise<OpenLocalArtifactResult> =>
      openLocalArtifact(taskId, action, artifactPath),
  );
}
