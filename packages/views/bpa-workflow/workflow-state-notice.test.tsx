// @vitest-environment jsdom
import { describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { WorkflowStateNotice } from "./workflow-state-notice";

describe("WorkflowStateNotice", () => {
  it("shows a concise approval card without internal workflow details", () => {
    render(
      <WorkflowStateNotice
        metadata={{
          "bpa.template": "production",
          "bpa.waiting_for": "human_approval",
          "bpa.approval_summary": "Дія: deploy. Вплив: нова revision.",
          "bpa.approval_fingerprint": "sha256:internal-only",
        }}
      />,
    );

    expect(screen.getByText("Потрібне погодження")).toBeTruthy();
    expect(screen.getByText("Дія: deploy. Вплив: нова revision.")).toBeTruthy();
    expect(screen.queryByText(/sha256/)).toBeNull();
  });

  it("sends an explicit approval decision", () => {
    const onDecision = vi.fn();
    render(
      <WorkflowStateNotice
        metadata={{
          "bpa.template": "production",
          "bpa.waiting_for": "human_approval",
          "bpa.approval_summary": "Підтвердити безпечний rollout.",
        }}
        onDecision={onDecision}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Погодити" }));
    expect(onDecision).toHaveBeenCalledWith("approved");
  });
});
