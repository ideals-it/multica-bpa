import { describe, expect, it } from "vitest";
import { mkdir, realpath, symlink, writeFile } from "fs/promises";
import { join } from "path";
import { tmpdir } from "os";
import { mkdtemp } from "fs/promises";
import { resolveLocalArtifactPath } from "./local-artifact";

describe("resolveLocalArtifactPath", () => {
  it("returns a real artifact inside the recorded work directory", async () => {
    const workDir = await mkdtemp(join(tmpdir(), "multica-artifact-"));
    await mkdir(join(workDir, "audit-results"));
    await writeFile(join(workDir, "audit-results", "report.json"), "{}", "utf8");

    await expect(
      resolveLocalArtifactPath(workDir, "audit-results/report.json"),
    ).resolves.toBe(await realpath(join(workDir, "audit-results", "report.json")));
  });

  it("rejects traversal and a symlink that escapes the recorded work directory", async () => {
    const workDir = await mkdtemp(join(tmpdir(), "multica-artifact-"));
    const outside = await mkdtemp(join(tmpdir(), "multica-outside-"));
    await writeFile(join(outside, "secret.json"), "{}", "utf8");
    await symlink(join(outside, "secret.json"), join(workDir, "escaped.json"));

    await expect(resolveLocalArtifactPath(workDir, "../secret.json")).rejects.toThrow(
      "relative",
    );
    await expect(resolveLocalArtifactPath(workDir, "escaped.json")).rejects.toThrow(
      "outside",
    );
  });
});
